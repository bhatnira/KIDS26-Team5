package agent

import (
	"testing"

	agentmod "antelope/internal/modules/agent"

	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

// toolResultChoice builds one role=tool choice carrying a
// workspace_save_artifact result for the given logical name.
func toolResultChoice(id, savedAs string) model.Choice {
	return model.Choice{
		Message: model.Message{
			Role:     model.RoleTool,
			ToolID:   id,
			ToolName: "workspace_save_artifact",
			Content:  `{"saved_as":"` + savedAs + `","version":0,"mime_type":"application/octet-stream","size_bytes":10}`,
		},
	}
}

// TestSerializeEventsParallelToolResults guards the parallel-tool-call
// regression: the framework merges concurrent tool calls into ONE event with
// one choice per result (mergeParallelToolCallResponseEvents). serializeEvents
// must emit a tool_result + artifact for EVERY choice, not just Choices[0] —
// otherwise all but the first parallel workspace_save_artifact card vanish on
// reload (the exact bug seen with models that batch save_artifact calls).
func TestSerializeEventsParallelToolResults(t *testing.T) {
	merged := event.Event{
		Response: &model.Response{
			Object: model.ObjectTypeToolResponse,
			Choices: []model.Choice{
				toolResultChoice("call_1", "out/filtered.h5ad"),
				toolResultChoice("call_2", "out/qc_metrics.csv"),
				toolResultChoice("call_3", "out/qc_violins.png"),
			},
		},
	}
	sess := session.NewSession("app", "1", "s1", session.WithSessionEvents([]event.Event{merged}))

	msgs := serializeEvents(sess)

	var toolResults, artifacts []string
	for _, m := range msgs {
		switch m["role"] {
		case "tool_result":
			toolResults = append(toolResults, m["tool_id"].(string))
		case "artifact":
			artifacts = append(artifacts, m["saved_as"].(string))
		}
	}

	if len(toolResults) != 3 {
		t.Errorf("got %d tool_result entries, want 3: %v", len(toolResults), toolResults)
	}
	wantArts := []string{"out/filtered.h5ad", "out/qc_metrics.csv", "out/qc_violins.png"}
	if len(artifacts) != len(wantArts) {
		t.Fatalf("got %d artifact cards, want %d: %v", len(artifacts), len(wantArts), artifacts)
	}
	for i, w := range wantArts {
		if artifacts[i] != w {
			t.Errorf("artifact %d = %q, want %q", i, artifacts[i], w)
		}
	}
}

func TestMergeStagedAttachments(t *testing.T) {
	prior := []agentmod.AttachmentRef{
		{Bucket: "b", Key: "u/1/a.csv", Name: "a.csv", StagedPath: "work/inputs/a.csv"},
		{Bucket: "b", Key: "u/1/b.csv", Name: "b.csv", StagedPath: "work/inputs/b.csv"},
	}
	current := []agentmod.AttachmentRef{
		// Re-attached at the same path: must replace the prior entry.
		{Bucket: "b", Key: "u/2/a.csv", Name: "a.csv", StagedPath: "work/inputs/a.csv"},
		{Bucket: "b", Key: "u/2/c.csv", Name: "c.csv", StagedPath: "work/inputs/c.csv"},
		// Not stageable: must not be persisted.
		{Bucket: "b", Key: "u/2/gone.csv", Name: "gone.csv", StagedPath: "work/inputs/gone.csv", Unavailable: true},
		{Bucket: "b", Key: "u/2/nopath.csv", Name: "nopath.csv"},
	}

	got := mergeStagedAttachments(prior, current)
	want := []struct{ path, key string }{
		{"work/inputs/b.csv", "u/1/b.csv"},
		{"work/inputs/a.csv", "u/2/a.csv"},
		{"work/inputs/c.csv", "u/2/c.csv"},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i].StagedPath != w.path || got[i].Key != w.key {
			t.Errorf("entry %d = {path:%q key:%q}, want {path:%q key:%q}",
				i, got[i].StagedPath, got[i].Key, w.path, w.key)
		}
	}
}

func TestMergeStagedAttachmentsEmptyInputs(t *testing.T) {
	if got := mergeStagedAttachments(nil, nil); len(got) != 0 {
		t.Fatalf("expected empty merge, got %+v", got)
	}
	prior := []agentmod.AttachmentRef{
		{Bucket: "b", Key: "k", Name: "a.csv", StagedPath: "work/inputs/a.csv"},
	}
	got := mergeStagedAttachments(prior, nil)
	if len(got) != 1 || got[0].Key != "k" {
		t.Fatalf("prior-only merge wrong: %+v", got)
	}
}
