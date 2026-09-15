//go:build skills

package skillbundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file only builds with the `skills` tag, i.e. exactly the configuration
// release and container builds use. It proves the whole embedded path hangs
// together: the //go:embed directive found a real bundle, the index round-trips,
// extraction lands a usable tree on disk, and the framework repository can stage
// a skill out of it. Run with:
//
//	go test -tags skills ./internal/modules/agent/skillbundle/
func TestEmbeddedBundleOpensAndStages(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "cache")

	lib, err := Open(Options{ExtractRoot: dest})
	if err != nil {
		t.Fatalf("opening the embedded bundle failed: %v", err)
	}
	if lib == nil {
		t.Fatal("a build tagged `skills` must carry a bundle")
	}
	if !lib.Embedded() {
		t.Error("library should report itself embedded")
	}
	if !lib.Extracted() {
		t.Error("first open into an empty dir should have unpacked the bundle")
	}
	if lib.Root() != dest {
		t.Errorf("root = %q, want the configured extract dir %q", lib.Root(), dest)
	}
	if lib.Len() == 0 {
		t.Fatal("embedded bundle is empty")
	}

	// Provenance has to survive into the image: MIT redistribution needs the
	// licence text to travel with the copy.
	for _, src := range lib.Catalog().Sources() {
		if src.License == "" {
			t.Errorf("source %q records no licence", src.Name)
		}
		if _, err := os.Stat(filepath.Join(dest, src.Name, "LICENSE")); err != nil {
			t.Errorf("source %q ships no LICENSE in the extracted bundle: %v", src.Name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, NoticeFile)); err != nil {
		t.Errorf("extracted bundle has no %s: %v", NoticeFile, err)
	}

	// Every catalogued skill must be resolvable to a real directory, because
	// that is what the framework copies into the sandbox. A catalog entry with
	// no directory would surface as a skill the model can find but never run.
	repo := lib.Repository()
	for _, e := range lib.Catalog().Entries() {
		dir, err := repo.Path(e.Name)
		if err != nil {
			t.Fatalf("skill %q in the catalog has no directory: %v", e.Name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
			t.Fatalf("skill %q directory has no SKILL.md: %v", e.Name, err)
		}
	}

	// Loading one skill end to end.
	first := lib.Catalog().Entries()[0]
	sk, err := repo.Get(first.Name)
	if err != nil {
		t.Fatalf("Get(%q): %v", first.Name, err)
	}
	if strings.TrimSpace(sk.Body) == "" {
		t.Errorf("skill %q loaded with an empty body", first.Name)
	}
}

// A self-contained binary must not have its skill library silently replaced by
// whatever sits at the configured disk root on the host.
func TestEmbeddedBundleWinsOverDiskRoot(t *testing.T) {
	decoy := t.TempDir()
	lib, err := Open(Options{
		DiskRoot:    decoy,
		ExtractRoot: filepath.Join(t.TempDir(), "cache"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if lib == nil || !lib.Embedded() {
		t.Fatal("Open should have used the embedded bundle")
	}
	if lib.Root() == decoy {
		t.Error("DiskRoot took precedence over the embedded bundle")
	}
}

// The whole point of the compact renderer is that the prompt cost stays flat as
// the library grows. Assert that against the real bundle, not a fixture.
func TestEmbeddedBundleOverviewIsSmall(t *testing.T) {
	lib, err := Open(Options{ExtractRoot: filepath.Join(t.TempDir(), "cache")})
	if err != nil || lib == nil {
		t.Fatalf("open: %v", err)
	}
	repo := lib.Repository()
	overview := RenderOverview(lib.Catalog(), repo.Summaries(), "skill_search")

	// The framework's default renderer would emit every name and description.
	var verbatim int
	for _, s := range repo.Summaries() {
		verbatim += len(s.Name) + len(s.Description) + 4
	}
	// ~4 chars/token. 8k chars is ~2k tokens, against the ~100k tokens the
	// default would spend on the same library.
	const maxChars = 8000
	if len(overview) > maxChars {
		t.Errorf("overview is %d chars for %d skills, want under %d",
			len(overview), lib.Len(), maxChars)
	}
	t.Logf("overview %d chars (~%d tokens) vs %d chars (~%d tokens) listed verbatim, for %d skills",
		len(overview), len(overview)/4, verbatim, verbatim/4, lib.Len())
}
