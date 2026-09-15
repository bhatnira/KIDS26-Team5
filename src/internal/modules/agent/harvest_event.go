package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"

	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// harvestArtifactsTag marks the synthetic event the runner emits after the
// end-of-turn output harvest. It is both pushed onto the downstream output
// channel (for live SSE cards) and appended to the session (so the cards
// survive a conversation reload).
const harvestArtifactsTag = "antelope.harvest_artifacts"

// harvestArtifactsStateKey is the event StateDelta key carrying the structured
// artifact list. The structured payload rides in StateDelta — NOT in the
// message content — so it is invisible to the model when the persisted event
// is replayed as conversation history, while the UI (live translate + reload
// serialize) reads it back to render cards. The message Content holds only a
// short human summary, which is the only part the model ever sees.
const harvestArtifactsStateKey = "antelope.harvested_artifacts"

// Live-only progress events. Neither is persisted: they exist purely to drive
// the in-chat "syncing" placeholders while the harvest uploads files (which
// happens after the turn's text, so without them there is a blank gap).
//
//	pending  → emitted once after the in-sandbox listing (cheap) and before the
//	           slow uploads, so skeleton cards appear immediately.
//	settled  → emitted once after all uploads, naming the files that actually
//	           saved, so the UI can drop any placeholder whose upload failed.
const (
	artifactsPendingTag      = "antelope.artifacts_pending"
	artifactsPendingStateKey = "antelope.pending_artifacts"
	artifactsSettledTag      = "antelope.artifacts_settled"
	artifactsSettledStateKey = "antelope.settled_artifacts"
)

// PendingArtifact is the placeholder metadata known before a file is uploaded
// — enough to render a skeleton card. JSON tags match the SSE frame.
type PendingArtifact struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

// maxHarvestSummaryItems caps how many filenames the human summary lists
// before collapsing the rest into "(+N more)".
const maxHarvestSummaryItems = 10

// HarvestedArtifact is one file the output harvest persisted this turn. Its
// JSON tags match the frontend 'artifact' SSE event so the chat can render a
// card and the file browser can refresh, exactly as it did for the old
// workspace_save_artifact tool results.
type HarvestedArtifact struct {
	Name      string `json:"name"`
	SavedAs   string `json:"saved_as"`
	Version   int    `json:"version"`
	MimeType  string `json:"mime_type,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

// newHarvestEvent wraps the harvested artifacts in a tagged framework event:
// the structured list rides in StateDelta (read back by ParseHarvested for the
// UI, invisible to the model), and the assistant message Content is a short
// human summary (the only part the model sees on replay, and what satisfies
// the session's "valid content" persistence gate). Author is the agent name
// purely for traceability.
func newHarvestEvent(arts []HarvestedArtifact) *event.Event {
	data, err := json.Marshal(arts)
	if err != nil {
		return nil
	}
	rsp := &model.Response{
		Object: model.ObjectTypeChatCompletion,
		Done:   true,
		Choices: []model.Choice{{
			Message: model.Message{Role: model.RoleAssistant, Content: harvestSummary(arts)},
		}},
	}
	ev := event.NewResponseEvent("", agentName, rsp, event.WithTag(harvestArtifactsTag))
	ev.StateDelta = map[string][]byte{harvestArtifactsStateKey: data}
	return ev
}

// ParseHarvested returns the harvested artifacts carried by a harvest event,
// or (nil, false) for any other event. Both the live SSE translator and the
// reload serializer call it to turn the event into artifact cards. The list
// is read from StateDelta.
func ParseHarvested(ev *event.Event) ([]HarvestedArtifact, bool) {
	if ev == nil || !ev.ContainsTag(harvestArtifactsTag) {
		return nil, false
	}
	raw, ok := ev.StateDelta[harvestArtifactsStateKey]
	if !ok || len(raw) == 0 {
		return nil, false
	}
	var arts []HarvestedArtifact
	if err := json.Unmarshal(raw, &arts); err != nil {
		return nil, false
	}
	return arts, true
}

// newArtifactsPendingEvent announces the files about to be uploaded so the UI
// can show skeleton placeholders. Live-only (not persisted); the structured
// list rides in StateDelta and a short summary satisfies the content gate.
func newArtifactsPendingEvent(pending []PendingArtifact) *event.Event {
	data, err := json.Marshal(pending)
	if err != nil {
		return nil
	}
	rsp := &model.Response{
		Object: model.ObjectTypeChatCompletion,
		Done:   true,
		Choices: []model.Choice{{
			Message: model.Message{
				Role:    model.RoleAssistant,
				Content: fmt.Sprintf("Saving %d output file(s)…", len(pending)),
			},
		}},
	}
	ev := event.NewResponseEvent("", agentName, rsp, event.WithTag(artifactsPendingTag))
	ev.StateDelta = map[string][]byte{artifactsPendingStateKey: data}
	return ev
}

// ParsePending returns the pending artifacts carried by a pending event.
func ParsePending(ev *event.Event) ([]PendingArtifact, bool) {
	if ev == nil || !ev.ContainsTag(artifactsPendingTag) {
		return nil, false
	}
	raw, ok := ev.StateDelta[artifactsPendingStateKey]
	if !ok || len(raw) == 0 {
		return nil, false
	}
	var pending []PendingArtifact
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, false
	}
	return pending, true
}

// newArtifactsSettledEvent reports which files finished uploading so the UI
// can clear any still-pending placeholder (a failed upload). Live-only.
func newArtifactsSettledEvent(savedNames []string) *event.Event {
	data, err := json.Marshal(savedNames)
	if err != nil {
		return nil
	}
	rsp := &model.Response{
		Object: model.ObjectTypeChatCompletion,
		Done:   true,
		Choices: []model.Choice{{
			Message: model.Message{
				Role:    model.RoleAssistant,
				Content: fmt.Sprintf("Saved %d output file(s).", len(savedNames)),
			},
		}},
	}
	ev := event.NewResponseEvent("", agentName, rsp, event.WithTag(artifactsSettledTag))
	ev.StateDelta = map[string][]byte{artifactsSettledStateKey: data}
	return ev
}

// ParseSettled returns the saved filenames carried by a settled event.
func ParseSettled(ev *event.Event) ([]string, bool) {
	if ev == nil || !ev.ContainsTag(artifactsSettledTag) {
		return nil, false
	}
	raw, ok := ev.StateDelta[artifactsSettledStateKey]
	if !ok {
		return nil, false
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, false
	}
	return names, true
}

// harvestSummary renders the one-line note the model sees in history when a
// turn produced output files. It is purely informational (e.g. lets a
// follow-up turn reference "the file you just saved"); the UI never shows it.
func harvestSummary(arts []HarvestedArtifact) string {
	if len(arts) == 0 {
		return "No output files were produced this turn."
	}
	names := make([]string, 0, maxHarvestSummaryItems)
	for i, a := range arts {
		if i >= maxHarvestSummaryItems {
			break
		}
		names = append(names, fmt.Sprintf("%s (v%d)", path.Base(a.Name), a.Version))
	}
	list := strings.Join(names, ", ")
	if len(arts) > maxHarvestSummaryItems {
		list += fmt.Sprintf(" (+%d more)", len(arts)-maxHarvestSummaryItems)
	}
	return fmt.Sprintf("Saved %d output file(s) to the workspace: %s.", len(arts), list)
}

// ArtifactSink bridges the harvest (which runs in the factory's cleanup
// closure) and the runner across the turn context. It serves two roles:
//
//   - Collect: the harvest Adds each saved artifact so the runner can persist
//     one consolidated event after cleanup (for conversation reload).
//   - Emit: the runner installs a live emitter (sending onto the downstream
//     event channel) BEFORE cleanup runs, so the harvest can push progress
//     events (pending → per-file → settled) as it uploads.
//
// Safe for concurrent use — the harvest uploads files in parallel.
type ArtifactSink struct {
	mu   sync.Mutex
	arts []HarvestedArtifact
	emit func(*event.Event)
}

// Add appends harvested artifacts. No-op on a nil sink.
func (s *ArtifactSink) Add(arts ...HarvestedArtifact) {
	if s == nil || len(arts) == 0 {
		return
	}
	s.mu.Lock()
	s.arts = append(s.arts, arts...)
	s.mu.Unlock()
}

// Drain returns the collected artifacts and clears the slice.
func (s *ArtifactSink) Drain() []HarvestedArtifact {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	out := s.arts
	s.arts = nil
	s.mu.Unlock()
	return out
}

// SetEmitter installs the live progress emitter. The runner sets it once,
// before the harvest runs, after the downstream channel exists.
func (s *ArtifactSink) SetEmitter(fn func(*event.Event)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.emit = fn
	s.mu.Unlock()
}

// Emit sends a live progress event if an emitter is installed (no-op
// otherwise, e.g. when the client has disconnected). The emitter is read
// under lock but invoked outside it so a slow send never blocks Add/Drain.
func (s *ArtifactSink) Emit(ev *event.Event) {
	if s == nil || ev == nil {
		return
	}
	s.mu.Lock()
	fn := s.emit
	s.mu.Unlock()
	if fn != nil {
		fn(ev)
	}
}

// artifactSinkCtxKey carries the per-turn ArtifactSink.
type artifactSinkCtxKey struct{}

// WithArtifactSink stamps a sink onto ctx so the harvest closure can record
// what it saved.
func WithArtifactSink(ctx context.Context, sink *ArtifactSink) context.Context {
	return context.WithValue(ctx, artifactSinkCtxKey{}, sink)
}

// ArtifactSinkFromContext returns the sink set by WithArtifactSink, or nil.
func ArtifactSinkFromContext(ctx context.Context) *ArtifactSink {
	s, _ := ctx.Value(artifactSinkCtxKey{}).(*ArtifactSink)
	return s
}
