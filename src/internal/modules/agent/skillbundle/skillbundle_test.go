package skillbundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"trpc.group/trpc-go/trpc-agent-go/skill"
)

// fakeBundle writes a minimal but structurally real bundle to a temp dir.
func fakeBundle(t *testing.T, entries ...Entry) string {
	t.Helper()
	root := t.TempDir()
	for _, e := range entries {
		dir := filepath.Join(root, filepath.FromSlash(e.Dir))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		doc := "---\nname: " + e.Name + "\ndescription: " + e.Description + "\n---\n\nBody of " + e.Name + ".\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx := &Index{
		Version:     IndexVersion,
		GeneratedBy: "test",
		Hash:        "deadbeef",
		Skills:      entries,
		Source:      []Source{{Name: "test", License: "MIT", Skills: len(entries)}},
	}
	body, err := idx.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, IndexFile), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func sampleEntries() []Entry {
	return []Entry{
		{Name: "sc-qc", Description: "Quality-control single-cell counts", Domain: "single-cell", Dir: "s/sc-qc", Tags: []string{"scanpy"}},
		{Name: "sc-cluster", Description: "Cluster single-cell embeddings", Domain: "single-cell", Dir: "s/sc-cluster"},
		{Name: "bam-sort", Description: "Sort and index alignment files", Domain: "alignment", Dir: "s/bam-sort"},
		{Name: "variant-call", Description: "Call germline variants from a BAM", Domain: "variant-calling", Dir: "s/variant-call"},
	}
}

func TestCatalogDomainsAndLookup(t *testing.T) {
	c := NewCatalog(&Index{Version: IndexVersion, Skills: sampleEntries()})
	if c.Len() != 4 {
		t.Fatalf("Len = %d, want 4", c.Len())
	}
	domains := c.Domains()
	// Largest domain first so the compact overview leads with the useful ones.
	if domains[0].Domain != "single-cell" || domains[0].Skills != 2 {
		t.Errorf("domains[0] = %+v, want single-cell/2", domains[0])
	}
	if len(domains) != 3 {
		t.Errorf("got %d domains, want 3", len(domains))
	}
	if e, ok := c.Lookup("bam-sort"); !ok || e.Domain != "alignment" {
		t.Errorf("Lookup(bam-sort) = %+v, %v", e, ok)
	}
	if _, ok := c.Lookup("nope"); ok {
		t.Error("Lookup of an unknown skill should fail")
	}
}

func TestCatalogSearch(t *testing.T) {
	c := NewCatalog(&Index{Version: IndexVersion, Skills: sampleEntries()})

	cases := []struct {
		query string
		want  string
	}{
		{"sc-qc", "sc-qc"},                    // exact name
		{"quality control", "sc-qc"},          // description words
		{"sort bam", "bam-sort"},              // name beats a description mention
		{"germline variants", "variant-call"}, // plural stems to the singular
		{"cluster", "sc-cluster"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			hits := c.Search(tc.query, 5)
			if len(hits) == 0 {
				t.Fatalf("no hits for %q", tc.query)
			}
			if hits[0].Entry.Name != tc.want {
				names := make([]string, 0, len(hits))
				for _, h := range hits {
					names = append(names, h.Entry.Name)
				}
				t.Errorf("top hit = %q, want %q (ranked: %v)", hits[0].Entry.Name, tc.want, names)
			}
		})
	}

	if hits := c.Search("", 5); hits != nil {
		t.Error("empty query should return no hits")
	}
	if hits := c.Search("kubernetes helm chart", 5); len(hits) != 0 {
		t.Errorf("unrelated query matched %d skills", len(hits))
	}
	if hits := c.Search("single cell", 1); len(hits) != 1 {
		t.Errorf("limit not honoured: got %d hits", len(hits))
	}
}

func TestRepositoryServesSummariesFromCatalog(t *testing.T) {
	entries := sampleEntries()
	root := fakeBundle(t, entries...)
	lib, err := openDisk(root)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Len() != len(entries) {
		t.Fatalf("Len = %d, want %d", lib.Len(), len(entries))
	}
	repo := lib.Repository()
	if got := len(repo.Summaries()); got != len(entries) {
		t.Errorf("Summaries = %d, want %d", got, len(entries))
	}

	// Get and Path must reach the real files, since the framework stages a
	// skill by copying Path()'s directory into the sandbox.
	sk, err := repo.Get("sc-qc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sk.Body, "Body of sc-qc.") {
		t.Errorf("Get returned body %q", sk.Body)
	}
	dir, err := repo.Path("sc-qc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Errorf("Path %q is not a real skill directory: %v", dir, err)
	}
	if _, err := repo.Path("missing-skill"); err == nil {
		t.Error("Path of an unknown skill should fail")
	}
}

// A dev checkout with no generated bundle must not stop the server from
// starting; the agent just has no built-in skills.
func TestOpenMissingBundleIsNotAnError(t *testing.T) {
	lib, err := openDisk(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("missing bundle should not error: %v", err)
	}
	if lib != nil {
		t.Fatalf("expected no library, got %+v", lib)
	}
	// Every accessor must tolerate the nil library.
	if lib.Len() != 0 || lib.Root() != "" || lib.Catalog() != nil || lib.Repository() != nil {
		t.Error("nil library accessors must be safe and empty")
	}
	if lib.Embedded() || lib.Extracted() {
		t.Error("nil library should report neither embedded nor extracted")
	}
}

// An index promising more skills than the tree holds means a truncated or
// hand-edited bundle; serving it would advertise skills that cannot be staged.
func TestOpenRejectsIncompleteBundle(t *testing.T) {
	root := fakeBundle(t, sampleEntries()...)
	if err := os.RemoveAll(filepath.Join(root, "s", "bam-sort")); err != nil {
		t.Fatal(err)
	}
	_, err := openDisk(root)
	if err == nil {
		t.Fatal("expected an error for an incomplete bundle")
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("error = %v, want it to mention the bundle is incomplete", err)
	}
}

func TestOpenRejectsUnsupportedIndexVersion(t *testing.T) {
	root := fakeBundle(t, sampleEntries()...)
	body := `{"version": 99, "skills": []}`
	if err := os.WriteFile(filepath.Join(root, IndexFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openDisk(root); err == nil {
		t.Fatal("expected an error for an unsupported index version")
	}
}

func TestExtractIsHashGatedAndRestoresExecutableBit(t *testing.T) {
	src := fstest.MapFS{
		IndexFile:      &fstest.MapFile{Data: []byte(`{"version":1,"hash":"abc123","skills":[]}`)},
		"s/x/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: x\ndescription: d\n---\n")},
		"s/x/run.sh":   &fstest.MapFile{Data: []byte("#!/bin/sh\necho hi\n")},
		"s/x/notes.md": &fstest.MapFile{Data: []byte("plain\n")},
	}
	dest := filepath.Join(t.TempDir(), "extracted")

	wrote, err := extract(src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("first extraction should write")
	}
	// //go:embed loses mode bits, so the shebang rule has to restore them or
	// a skill's own run.sh stops being runnable.
	info, err := os.Stat(filepath.Join(dest, "s", "x", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("run.sh mode = %v, want the executable bit set", info.Mode().Perm())
	}
	plain, err := os.Stat(filepath.Join(dest, "s", "x", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if plain.Mode().Perm()&0o111 != 0 {
		t.Errorf("notes.md mode = %v, want no executable bit", plain.Mode().Perm())
	}

	// Second run must be a no-op: restarts should not rewrite ~23 MiB.
	wrote, err = extract(src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Error("second extraction rewrote an up-to-date bundle")
	}

	// A stale tree is replaced, not merged, so removals propagate.
	if err := os.WriteFile(filepath.Join(dest, stampFile), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "leftover.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if wrote, err = extract(src, dest); err != nil || !wrote {
		t.Fatalf("stale bundle should be re-extracted: wrote=%v err=%v", wrote, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "leftover.md")); err == nil {
		t.Error("re-extraction must not leave stale files behind")
	}
}

func TestRenderOverviewStaysCompact(t *testing.T) {
	entries := sampleEntries()
	c := NewCatalog(&Index{Version: IndexVersion, Skills: entries})
	summaries := make([]skill.Summary, 0, len(entries)+1)
	for _, e := range entries {
		summaries = append(summaries, skill.Summary{Name: e.Name, Description: e.Description})
	}
	summaries = append(summaries, skill.Summary{Name: "my-skill", Description: "A skill I uploaded"})

	out := RenderOverview(c, summaries, "skill_search")

	if !strings.HasPrefix(out, overviewHeader) {
		t.Errorf("overview must open with %q so the framework recognises the section", overviewHeader)
	}
	// Built-in descriptions are exactly what must NOT be inlined.
	if strings.Contains(out, "Quality-control single-cell counts") {
		t.Error("built-in descriptions leaked into the overview")
	}
	if !strings.Contains(out, "single-cell(2)") {
		t.Errorf("domain counts missing from overview:\n%s", out)
	}
	if !strings.Contains(out, "skill_search") {
		t.Error("overview must point the model at the search tool")
	}
	// User skills are few and expected to be known without asking, so they
	// stay listed in full.
	if !strings.Contains(out, "- my-skill: A skill I uploaded") {
		t.Errorf("user skill not listed in full:\n%s", out)
	}
}

func TestRenderOverviewWithNoSkills(t *testing.T) {
	out := RenderOverview(nil, nil, "skill_search")
	if !strings.Contains(out, "No skills are available") {
		t.Errorf("empty overview should say so explicitly, got:\n%s", out)
	}
}

func TestMultiPrefersFirstRepository(t *testing.T) {
	builtinRoot := fakeBundle(t, Entry{Name: "shared", Description: "built-in version", Dir: "s/shared"})
	lib, err := openDisk(builtinRoot)
	if err != nil {
		t.Fatal(err)
	}

	userRoot := t.TempDir()
	dir := filepath.Join(userRoot, "shared")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "---\nname: shared\ndescription: user version\n---\n\nUser body.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	userRepo, err := UserRepository(userRoot)
	if err != nil {
		t.Fatal(err)
	}
	if userRepo == nil {
		t.Fatal("user repository should have been built")
	}

	multi := NewMulti(lib.Repository(), userRepo)
	sums := multi.Summaries()
	if len(sums) != 1 {
		t.Fatalf("Summaries = %+v, want the duplicate collapsed", sums)
	}
	if sums[0].Description != "built-in version" {
		t.Errorf("precedence broken: got %q", sums[0].Description)
	}

	// Composing nothing yields an untyped nil so callers can test it.
	if NewMulti(nil, nil) != nil {
		t.Error("NewMulti of nils should be nil")
	}
	if got := NewMulti(userRepo); got != userRepo {
		t.Error("NewMulti of one repository should return it unwrapped")
	}
}

func TestUserRepositoryEmptyDirs(t *testing.T) {
	repo, err := UserRepository(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if repo != nil {
		t.Error("a directory with no skills should yield no repository")
	}
	if repo, err = UserRepository(); err != nil || repo != nil {
		t.Errorf("no dirs should yield no repository: %v, %v", repo, err)
	}
}
