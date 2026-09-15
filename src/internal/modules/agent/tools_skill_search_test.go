package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"antelope/internal/modules/agent/skillbundle"
)

func testCatalog() *skillbundle.Catalog {
	return skillbundle.NewCatalog(&skillbundle.Index{
		Version: skillbundle.IndexVersion,
		Skills: []skillbundle.Entry{
			{Name: "sc-qc", Description: "Quality-control single-cell counts", Domain: "single-cell", Dir: "s/sc-qc"},
			{Name: "sc-cluster", Description: "Cluster single-cell embeddings", Domain: "single-cell", Dir: "s/sc-cluster"},
			{Name: "bam-sort", Description: "Sort and index alignment files", Domain: "alignment", Dir: "s/bam-sort"},
			{Name: "somatic-calling", Description: "Call somatic variants from a tumour BAM", Domain: "variant-calling", Dir: "s/somatic-calling"},
		},
	})
}

func callSearch(t *testing.T, in map[string]any) skillSearchOutput {
	t.Helper()
	tool := NewSkillSearchTool(testCatalog())
	if tool == nil {
		t.Fatal("tool should be registered for a non-empty catalog")
	}
	args, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tool.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("Call(%v): %v", in, err)
	}
	out, ok := got.(skillSearchOutput)
	if !ok {
		t.Fatalf("Call returned %T, want skillSearchOutput", got)
	}
	return out
}

func TestSkillSearchToolRanksAndLimits(t *testing.T) {
	out := callSearch(t, map[string]any{"query": "somatic variants tumour"})
	if len(out.Hits) == 0 {
		t.Fatal("no hits")
	}
	if out.Hits[0].Name != "somatic-calling" {
		t.Errorf("top hit = %q, want somatic-calling", out.Hits[0].Name)
	}
	if out.Hits[0].Domain != "variant-calling" {
		t.Errorf("hit is missing its domain: %+v", out.Hits[0])
	}

	out = callSearch(t, map[string]any{"query": "single cell", "limit": 1})
	if len(out.Hits) != 1 {
		t.Errorf("limit ignored: got %d hits", len(out.Hits))
	}
	// Truncation has to be stated, or the model reads a partial list as the
	// whole library.
	if out.Note == "" {
		t.Error("truncated results should carry a note")
	}
}

func TestSkillSearchToolDomainFilter(t *testing.T) {
	// Domain alone lists that domain.
	out := callSearch(t, map[string]any{"domain": "single-cell"})
	if len(out.Hits) != 2 {
		t.Errorf("listing a domain returned %d hits, want 2", len(out.Hits))
	}

	// Domain plus query filters the ranked results.
	out = callSearch(t, map[string]any{"query": "cluster", "domain": "single-cell"})
	if len(out.Hits) != 1 || out.Hits[0].Name != "sc-cluster" {
		t.Errorf("filtered search = %+v, want just sc-cluster", out.Hits)
	}

	// A domain that does not exist must say so rather than return nothing and
	// look like an empty library.
	out = callSearch(t, map[string]any{"domain": "proteomics"})
	if len(out.Hits) != 0 || !strings.Contains(out.Note, "No domain") {
		t.Errorf("unknown domain = %+v, note %q", out.Hits, out.Note)
	}
}

func TestSkillSearchToolNoMatchExplainsItself(t *testing.T) {
	out := callSearch(t, map[string]any{"query": "kubernetes ingress"})
	if len(out.Hits) != 0 {
		t.Errorf("unrelated query matched: %+v", out.Hits)
	}
	if out.Note == "" {
		t.Error("an empty result must explain itself so the model stops guessing")
	}
}

func TestSkillSearchToolRequiresQueryOrDomain(t *testing.T) {
	tool := NewSkillSearchTool(testCatalog())
	if _, err := tool.Call(context.Background(), []byte(`{}`)); err == nil {
		t.Error("expected an error with neither query nor domain")
	}
	if _, err := tool.Call(context.Background(), []byte(`not json`)); err == nil {
		t.Error("expected an error for malformed args")
	}
}

// With no built-in library the tool must not be offered at all, rather than
// advertised and always empty.
func TestSkillSearchToolAbsentWithoutCatalog(t *testing.T) {
	if NewSkillSearchTool(nil) != nil {
		t.Error("nil catalog should yield no tool")
	}
	empty := skillbundle.NewCatalog(&skillbundle.Index{Version: skillbundle.IndexVersion})
	if NewSkillSearchTool(empty) != nil {
		t.Error("empty catalog should yield no tool")
	}
}

// The system prompt teaches the search-then-load protocol by name. If the tool
// is ever renamed, the prompt has to move with it.
func TestSystemInstructionTeachesTheSearchTool(t *testing.T) {
	if !strings.Contains(SystemInstruction, SkillSearchToolName) {
		t.Errorf("SystemInstruction does not mention %q", SkillSearchToolName)
	}
	decl := NewSkillSearchTool(testCatalog()).Declaration()
	if decl.Name != SkillSearchToolName {
		t.Errorf("tool declares name %q, want %q", decl.Name, SkillSearchToolName)
	}
}
