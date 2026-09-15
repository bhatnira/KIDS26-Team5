package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	agentmod "antelope/internal/modules/agent"
	"antelope/internal/modules/log"
	"antelope/internal/modules/sse"

	"go.uber.org/zap"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// stagingHeartbeatInterval paces the keepalive pings emitted while the
// runner is still staging attachments (which can take minutes for large
// files). Without them the SSE connection is byte-silent and reverse
// proxies may kill it as idle.
const stagingHeartbeatInterval = 15 * time.Second

// agentSSESource bridges the framework event channel (returned by
// agent.Manager.Run) to the SSE wire schema the frontend speaks.
//
// Wire event map:
//
//	token             → assistant delta text
//	reasoning         → assistant thinking/reasoning delta text (foldable in UI)
//	tool_call         → function call arguments emitted by the model
//	tool_result       → tool execution output
//	code_exec         → code block the framework is about to execute
//	code_result       → executed code stdout/stderr + output files
//	artifact          → artifact saved by a tool or the output harvest
//	artifacts_pending → output files about to upload (skeleton placeholders)
//	artifacts_settled → output files that finished uploading (clear failures)
//	skill_loaded      → skill_select_docs / skill_load completion
//	error             → terminal error
//	done              → end-of-stream marker
type agentSSESource struct {
	mgr         *agentmod.Manager
	userID      uint
	sessionID   string
	message     model.Message
	attachments []agentmod.AttachmentRef
	connID      string

	// initialTitle is set by the chat service when a default-titled
	// conversation got auto-named on this turn. The SSE source emits a
	// 'title_updated' event so the frontend conversation list refreshes
	// in-place without an extra round-trip.
	initialTitle string

	// streamedContent / streamedReasoning record whether the CURRENT assistant
	// message already had its answer/reasoning text emitted as streaming deltas.
	// In streaming mode the framework follows the per-token chunks with a final
	// aggregated ObjectTypeChatCompletion event that repeats the FULL accumulated
	// text; without these guards we'd emit that text a second time and the
	// frontend — which appends every 'token'/'reasoning' frame — would render the
	// whole message twice (the double-render seen live but not after a reload,
	// which reads the single persisted copy). Reset at each assistant-message
	// boundary (a tool-call turn or the final message). Only ever mutated from
	// the single-goroutine Stream loop, so no synchronization is needed.
	streamedContent   bool
	streamedReasoning bool
}

func newAgentSSESource(mgr *agentmod.Manager, userID uint, sessionID string, msg model.Message, attachments []agentmod.AttachmentRef) *agentSSESource {
	return &agentSSESource{
		mgr:         mgr,
		userID:      userID,
		sessionID:   sessionID,
		message:     msg,
		attachments: attachments,
		connID:      fmt.Sprintf("agent-%d-%s-%d", userID, sessionID, time.Now().UnixNano()),
	}
}

func (s *agentSSESource) ConnectionID() string { return s.connID }

// Stream blocks until the runner emits a final event or the context is
// cancelled. Errors from individual event writes do not stop the stream
// unless the client disconnected.
func (s *agentSSESource) Stream(ctx context.Context, w *sse.Writer) error {
	// Surface the auto-generated conversation title as the first event so
	// the chat sidebar can swap "New Conversation" for the real title
	// immediately, before any LLM tokens arrive.
	if s.initialTitle != "" {
		_ = w.WriteJSON("title_updated", map[string]string{"title": s.initialTitle})
	}

	// mgr.Run blocks until the agent factory has finished — including
	// attachment staging, which for large files can take minutes. During
	// that window emit a status line plus periodic pings so the connection
	// is never byte-silent. The heartbeat goroutine is the ONLY writer
	// until stopHeartbeat returns (sse.Writer is not concurrency-safe), so
	// it must be stopped before any other write below.
	var hbStop chan struct{}
	var hbDone sync.WaitGroup
	if len(s.attachments) > 0 {
		_ = w.WriteJSON("status", map[string]string{
			"message": fmt.Sprintf("Preparing %d attached file(s)…", len(s.attachments)),
		})
		hbStop = make(chan struct{})
		hbDone.Go(func() {
			t := time.NewTicker(stagingHeartbeatInterval)
			defer t.Stop()
			for {
				select {
				case <-hbStop:
					return
				case <-ctx.Done():
					return
				case <-t.C:
					_ = w.WriteJSON("ping", map[string]any{})
				}
			}
		})
	}
	stopHeartbeat := func() {
		if hbStop != nil {
			close(hbStop)
			hbDone.Wait()
			hbStop = nil
		}
	}
	defer stopHeartbeat()

	events, err := s.mgr.Run(ctx, s.userID, s.sessionID, s.message, s.attachments)
	stopHeartbeat()
	if err != nil {
		_ = w.WriteJSON("error", map[string]string{"message": err.Error()})
		_ = w.WriteJSON("done", map[string]any{})
		return err
	}

	doneSent := false
	for ev := range events {
		if ev == nil {
			continue
		}
		s.translate(w, ev)
		if isCompletion(ev) {
			_ = w.WriteJSON("done", map[string]any{})
			doneSent = true
		}
	}
	if !doneSent {
		_ = w.WriteJSON("done", map[string]any{})
	}
	return nil
}

// translate converts one framework event into zero or more SSE frames.
func (s *agentSSESource) translate(w *sse.Writer, ev *event.Event) {
	if ev.Response != nil && ev.Response.Error != nil {
		_ = w.WriteJSON("error", map[string]string{
			"message": ev.Response.Error.Message,
			"type":    ev.Response.Error.Type,
		})
		return
	}

	// Token usage rides along on the final response of a turn (and possibly the
	// last streamed chunk). Emit it as its own frame — without returning — so it
	// drives the frontend context-usage meter while the event's content is still
	// processed below. prompt_tokens is the size of the full context last sent.
	if ev.Response != nil && ev.Response.Usage != nil && ev.Response.Usage.PromptTokens > 0 {
		_ = w.WriteJSON("usage", map[string]int{
			"prompt_tokens":     ev.Response.Usage.PromptTokens,
			"completion_tokens": ev.Response.Usage.CompletionTokens,
			"total_tokens":      ev.Response.Usage.TotalTokens,
		})
	}

	// Synthetic end-of-turn harvest event: emit one artifact frame per saved
	// output file so the chat renders result cards and the file browser
	// refreshes, mirroring the old workspace_save_artifact tool results.
	if arts, ok := agentmod.ParseHarvested(ev); ok {
		for _, a := range arts {
			_ = w.WriteJSON("artifact", a)
		}
		return
	}

	// Harvest progress (live-only): pending announces files about to upload so
	// the UI shows skeleton placeholders before the slow transfer; settled
	// lists what actually saved so the UI can drop failed placeholders.
	if pending, ok := agentmod.ParsePending(ev); ok {
		_ = w.WriteJSON("artifacts_pending", map[string]any{"items": pending})
		return
	}
	if saved, ok := agentmod.ParseSettled(ev); ok {
		_ = w.WriteJSON("artifacts_settled", map[string]any{"saved": saved})
		return
	}

	// Code execution lifecycle events use Tag.
	switch ev.Tag {
	case event.CodeExecutionTag:
		s.writeCodeExec(w, ev)
		return
	case event.CodeExecutionResultTag:
		s.writeCodeResult(w, ev)
		return
	}

	if ev.Response == nil || len(ev.Response.Choices) == 0 {
		return
	}
	choice := ev.Response.Choices[0]

	// Streaming delta reasoning ("thinking") text from the assistant. Emitted as
	// its own frame so the UI can fold it into a separate, muted block ahead of
	// the answer. Reasoning deltas arrive before the answer's content deltas.
	if ev.Response.Object == model.ObjectTypeChatCompletionChunk && choice.Delta.ReasoningContent != "" {
		s.streamedReasoning = true
		_ = w.WriteJSON("reasoning", map[string]string{"content": choice.Delta.ReasoningContent})
		return
	}

	// Streaming delta text from the assistant.
	if ev.Response.Object == model.ObjectTypeChatCompletionChunk && choice.Delta.Content != "" {
		s.streamedContent = true
		_ = w.WriteJSON("token", map[string]string{"content": choice.Delta.Content})
		return
	}

	// Tool calls emitted by the assistant.
	if len(choice.Message.ToolCalls) > 0 {
		for _, tc := range choice.Message.ToolCalls {
			_ = w.WriteJSON("tool_call", map[string]any{
				"id":        tc.ID,
				"name":      tc.Function.Name,
				"arguments": string(tc.Function.Arguments),
			})
			// Surface skill_load / skill_select_docs as a higher-level
			// skill_loaded event for the UI's "Loaded skill" pill.
			if isSkillLoadCall(tc.Function.Name) {
				_ = w.WriteJSON("skill_loaded", map[string]any{
					"name":      tc.Function.Name,
					"arguments": string(tc.Function.Arguments),
				})
			}
		}
		// This assistant message ended by calling tools; the next message (after
		// the tool results) is judged on its own deltas.
		s.endAssistantMessage()
		return
	}

	// Tool result(s) from the framework (role=tool). When the model issues
	// parallel tool calls, the framework merges every result into a SINGLE
	// event carrying one choice per tool result (see
	// mergeParallelToolCallResponseEvents in trpc-agent-go). Emit a frame for
	// every tool-result choice — not just Choices[0] — or all but the first
	// parallel result (e.g. extra workspace_save_artifact cards) are silently
	// dropped.
	if choice.Message.Role == model.RoleTool {
		for i := range ev.Response.Choices {
			msg := ev.Response.Choices[i].Message
			if msg.Role != model.RoleTool {
				continue
			}
			_ = w.WriteJSON("tool_result", map[string]any{
				"id":      msg.ToolID,
				"name":    msg.ToolName,
				"content": msg.Content,
			})
			s.emitArtifactEvents(w, msg.ToolName, msg.Content)
		}
		return
	}

	// Final assistant message (non-streaming or buffered). Reasoning, if any,
	// rides on the same message and is emitted first so it precedes the answer.
	if ev.Response.Object == model.ObjectTypeChatCompletion {
		// Only emit text that wasn't already streamed as deltas. In streaming
		// mode the deltas already carried the full message, so re-emitting here
		// would double-render it; in non-streaming mode no deltas were sent, so
		// the flags are false and this is the sole emission.
		if choice.Message.ReasoningContent != "" && !s.streamedReasoning {
			_ = w.WriteJSON("reasoning", map[string]string{"content": choice.Message.ReasoningContent})
		}
		if choice.Message.Content != "" && !s.streamedContent {
			_ = w.WriteJSON("token", map[string]string{"content": choice.Message.Content})
		}
		s.endAssistantMessage()
	}
}

// endAssistantMessage marks the boundary between one assistant message and the
// next within a single stream. It clears the streamed-text flags so the next
// message's final ObjectTypeChatCompletion event is judged solely on its own
// deltas. Call this from every branch that terminates an assistant message (it
// ends either by calling tools or by producing a final answer); forgetting to
// would let a stale flag suppress the next message's content.
func (s *agentSSESource) endAssistantMessage() {
	s.streamedContent = false
	s.streamedReasoning = false
}

// writeCodeExec emits the code-execution-start event.
func (s *agentSSESource) writeCodeExec(w *sse.Writer, ev *event.Event) {
	payload := map[string]any{"language": "python"}
	if ev.Response != nil && len(ev.Response.Choices) > 0 {
		payload["code"] = ev.Response.Choices[0].Message.Content
	}
	_ = w.WriteJSON("code_exec", payload)
}

// writeCodeResult emits the code-execution-result event including output
// files when the framework attaches them via the response payload.
func (s *agentSSESource) writeCodeResult(w *sse.Writer, ev *event.Event) {
	payload := map[string]any{}
	if ev.Response != nil && len(ev.Response.Choices) > 0 {
		payload["stdout"] = ev.Response.Choices[0].Message.Content
	}
	_ = w.WriteJSON("code_result", payload)
}

// emitArtifactEvents inspects known artifact-producing tool results (the
// inline python tool + the framework's workspace_save_artifact) and emits
// a structured artifact event so the frontend can render a preview/card
// without waiting for the next list_artifacts call.
func (s *agentSSESource) emitArtifactEvents(w *sse.Writer, toolName, contentJSON string) {
	switch toolName {
	case "run_python_inline":
		var res struct {
			Artifacts []map[string]any `json:"artifacts"`
		}
		if err := json.Unmarshal([]byte(contentJSON), &res); err != nil {
			log.L().Debug("parse run_python_inline result failed", zap.Error(err))
			return
		}
		for _, a := range res.Artifacts {
			_ = w.WriteJSON("artifact", a)
		}
	case "workspace_save_artifact", "antelope_save_artifact":
		var res map[string]any
		if err := json.Unmarshal([]byte(contentJSON), &res); err != nil {
			return
		}
		_ = w.WriteJSON("artifact", res)
	}
}

// isCompletion is true for the synthetic event the runner emits when an
// invocation is finished.
func isCompletion(ev *event.Event) bool {
	return ev.Response != nil && ev.Response.Object == model.ObjectTypeRunnerCompletion
}

func isSkillLoadCall(name string) bool {
	switch strings.TrimSpace(name) {
	case "skill_load", "skill_select_docs":
		return true
	}
	return false
}
