package agent

import (
	"strings"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/event"
)

func TestHarvestEventRoundTrip(t *testing.T) {
	in := []HarvestedArtifact{
		{Name: "out/qc_violins.png", SavedAs: "out/qc_violins.png", Version: 0, MimeType: "image/png", SizeBytes: 50688},
		{Name: "out/qc_metrics.csv", SavedAs: "out/qc_metrics.csv", Version: 2, MimeType: "text/csv", SizeBytes: 19456},
	}
	ev := newHarvestEvent(in)
	if ev == nil {
		t.Fatal("newHarvestEvent returned nil")
	}
	got, ok := ParseHarvested(ev)
	if !ok {
		t.Fatal("ParseHarvested did not recognize the harvest event")
	}
	if len(got) != len(in) {
		t.Fatalf("got %d artifacts, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("artifact %d = %+v, want %+v", i, got[i], in[i])
		}
	}

	// The structured payload must live in StateDelta (UI-facing, invisible to
	// the model), never in the message content the model sees on replay.
	if _, ok := ev.StateDelta[harvestArtifactsStateKey]; !ok {
		t.Error("structured artifacts missing from StateDelta")
	}
	content := ev.Response.Choices[0].Message.Content
	if strings.Contains(content, "size_bytes") || strings.Contains(content, "{") {
		t.Errorf("message content leaked structured JSON to the model: %q", content)
	}
	if !strings.Contains(content, "qc_violins.png") {
		t.Errorf("human summary should name the saved files, got %q", content)
	}
}

func TestParseHarvestedIgnoresOtherEvents(t *testing.T) {
	// A plain event without the harvest tag must not be misread.
	if _, ok := ParseHarvested(event.New("inv", "author")); ok {
		t.Error("ParseHarvested matched an untagged event")
	}
	if _, ok := ParseHarvested(nil); ok {
		t.Error("ParseHarvested matched a nil event")
	}
}

func TestPendingAndSettledRoundTrip(t *testing.T) {
	pendingIn := []PendingArtifact{
		{Name: "out/a.png", SizeBytes: 100},
		{Name: "out/b.csv", SizeBytes: 200},
	}
	pev := newArtifactsPendingEvent(pendingIn)
	if pev == nil {
		t.Fatal("newArtifactsPendingEvent returned nil")
	}
	gotPending, ok := ParsePending(pev)
	if !ok || len(gotPending) != 2 || gotPending[0] != pendingIn[0] {
		t.Fatalf("ParsePending round-trip failed: ok=%v got=%+v", ok, gotPending)
	}
	// A pending event is not a harvest or settled event.
	if _, ok := ParseHarvested(pev); ok {
		t.Error("pending event misread as harvested")
	}
	if _, ok := ParseSettled(pev); ok {
		t.Error("pending event misread as settled")
	}

	sev := newArtifactsSettledEvent([]string{"out/a.png", "out/b.csv"})
	if sev == nil {
		t.Fatal("newArtifactsSettledEvent returned nil")
	}
	gotSaved, ok := ParseSettled(sev)
	if !ok || len(gotSaved) != 2 || gotSaved[1] != "out/b.csv" {
		t.Fatalf("ParseSettled round-trip failed: ok=%v got=%+v", ok, gotSaved)
	}
	// An empty settled list (every upload failed) must still parse as settled
	// so the UI clears its placeholders.
	empty := newArtifactsSettledEvent(nil)
	if names, ok := ParseSettled(empty); !ok || len(names) != 0 {
		t.Errorf("empty settled event should parse to an empty slice, got ok=%v names=%+v", ok, names)
	}
}

func TestArtifactSinkEmit(t *testing.T) {
	var s ArtifactSink
	// No emitter installed: Emit is a no-op, not a panic.
	s.Emit(newArtifactsSettledEvent(nil))

	var got []string
	s.SetEmitter(func(ev *event.Event) {
		if names, ok := ParseSettled(ev); ok {
			got = append(got, names...)
		}
	})
	s.Emit(newArtifactsSettledEvent([]string{"out/x.txt"}))
	if len(got) != 1 || got[0] != "out/x.txt" {
		t.Fatalf("emitter did not receive event: %+v", got)
	}
	// nil-safe.
	var np *ArtifactSink
	np.SetEmitter(func(*event.Event) {})
	np.Emit(nil)
}

func TestArtifactSinkAddDrain(t *testing.T) {
	var s ArtifactSink
	s.Add(HarvestedArtifact{Name: "out/a.txt"})
	s.Add(HarvestedArtifact{Name: "out/b.txt"}, HarvestedArtifact{Name: "out/c.txt"})
	got := s.Drain()
	if len(got) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(got))
	}
	if len(s.Drain()) != 0 {
		t.Error("Drain did not clear the sink")
	}
	// nil-safe.
	var np *ArtifactSink
	np.Add(HarvestedArtifact{Name: "x"})
	if np.Drain() != nil {
		t.Error("nil sink Drain should return nil")
	}
}
