package runner

import (
	"strings"
	"testing"
)

func TestRenderHCL_NoVars(t *testing.T) {
	tpl := `job "x" {}`
	got, err := RenderHCL(tpl, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != tpl {
		t.Fatalf("expected passthrough, got %q", got)
	}
}

func TestRenderHCL_InjectsLocals(t *testing.T) {
	tpl := `job "${local.job_id}" {}`
	got, err := RenderHCL(tpl, map[string]any{
		"job_id":      "nf-generic",
		"cores":       2,
		"datacenters": []string{"cab", "dc1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Must start with a locals block.
	if !strings.HasPrefix(got, "locals {\n") {
		t.Fatalf("missing locals prefix:\n%s", got)
	}
	// Must contain each key with its JSON-encoded value.
	for _, needle := range []string{
		`job_id = "nf-generic"`,
		`cores = 2`,
		`datacenters = ["cab","dc1"]`,
		`job "${local.job_id}" {}`,
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("missing %q in:\n%s", needle, got)
		}
	}
}

func TestRenderHCL_RejectsBadIdent(t *testing.T) {
	_, err := RenderHCL(`x`, map[string]any{"bad-name": 1})
	if err == nil {
		t.Fatal("expected error for invalid identifier")
	}
}

func TestRenderHCL_Deterministic(t *testing.T) {
	vars := map[string]any{"b": 2, "a": 1, "c": 3}
	a, _ := RenderHCL("x", vars)
	b, _ := RenderHCL("x", vars)
	if a != b {
		t.Fatal("output is not deterministic")
	}
	// "a" must appear before "b" before "c" (sorted).
	ia, ib, ic := strings.Index(a, "a ="), strings.Index(a, "b ="), strings.Index(a, "c =")
	if !(ia < ib && ib < ic) {
		t.Fatalf("keys not sorted:\n%s", a)
	}
}

func TestRegistry_DuplicateErrors(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&genericRunner{name: "foo"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&genericRunner{name: "foo"}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRegistry_EmptyName(t *testing.T) {
	if err := NewRegistry().Register(&genericRunner{name: ""}); err == nil {
		t.Fatal("expected empty-name error")
	}
}
