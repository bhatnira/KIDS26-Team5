package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSkill lays out a minimal skill directory under root and returns its path.
func writeSkill(t *testing.T, root, rel, doc string, extra map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skillFile), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range extra {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReadSkillFrontMatter(t *testing.T) {
	root := t.TempDir()
	doc := `---
name: sc-qc
description: |-
  Quality-control a single-cell matrix,
  dropping low-count cells.
metadata:
  domain: single-cell
  tags:
    - scanpy
    - qc
---

# sc-qc

Body text.
`
	dir := writeSkill(t, root, "transcriptomics/sc-qc", doc, nil)
	src := SourceConfig{Name: "demo", Root: ".", DomainFrom: domainFromFrontMatter}

	sk, err := readSkill(dir, "transcriptomics/sc-qc", src)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != "sc-qc" {
		t.Errorf("name = %q, want sc-qc", sk.Name)
	}
	// A block scalar must collapse to one line so it fits the overview and
	// the catalog.
	want := "Quality-control a single-cell matrix, dropping low-count cells."
	if sk.Description != want {
		t.Errorf("description = %q, want %q", sk.Description, want)
	}
	if sk.Domain != "single-cell" {
		t.Errorf("domain = %q, want single-cell (from metadata.domain)", sk.Domain)
	}
	if strings.Join(sk.Tags, ",") != "scanpy,qc" {
		t.Errorf("tags = %v, want [scanpy qc]", sk.Tags)
	}
}

func TestResolveDomainFromPathSegment(t *testing.T) {
	src := SourceConfig{Name: "bioskills", DomainFrom: domainFromPathSegment}
	if got := resolveDomain(src, "Variant Calling/joint-genotyping", nil); got != "variant-calling" {
		t.Errorf("domain = %q, want variant-calling", got)
	}
	// A skill sitting directly under the source root has no domain segment,
	// so it falls back to the source name rather than claiming itself as a
	// domain.
	if got := resolveDomain(src, "loose-skill", nil); got != "bioskills" {
		t.Errorf("domain = %q, want bioskills", got)
	}
}

func TestResolveDomainFrontMatterFallbacks(t *testing.T) {
	src := SourceConfig{Name: "clawbio", DomainFrom: domainFromFrontMatter}
	cases := []struct {
		name string
		fm   frontMatter
		want string
	}{
		{"nested metadata", frontMatter{"metadata": map[string]any{"domain": "Genomics"}}, "genomics"},
		{"top-level domain", frontMatter{"domain": "proteomics"}, "proteomics"},
		{"category", frontMatter{"category": "single cell"}, "single-cell"},
		{"none", frontMatter{"license": "MIT"}, "clawbio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveDomain(src, "affinity-proteomics", tc.fm); got != tc.want {
				t.Errorf("domain = %q, want %q", got, tc.want)
			}
		})
	}
}

// A copyright banner above the YAML makes strict readers (including the
// framework's own repository) see no front matter at all. The bundler must move
// the banner below the front matter, keeping it verbatim.
func TestNormalizeDocHoistsFrontMatterAboveBanner(t *testing.T) {
	root := t.TempDir()
	doc := `<!--
# COPYRIGHT NOTICE
# Copyright (c) 2026 Example Author
-->

---
name: banner-skill
description: Does a thing.
---

# banner-skill
`
	dir := writeSkill(t, root, "banner-skill", doc, nil)
	src := SourceConfig{Name: "demo", Root: ".", DomainFrom: domainFromFrontMatter}

	sk, err := readSkill(dir, "banner-skill", src)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != "banner-skill" || sk.Description != "Does a thing." {
		t.Fatalf("front matter not recovered: name=%q desc=%q", sk.Name, sk.Description)
	}
	got := string(sk.Doc)
	if !strings.HasPrefix(got, "---\nname: banner-skill\n") {
		t.Errorf("normalised doc must open with front matter, got:\n%s", got[:min(120, len(got))])
	}
	if !strings.Contains(got, "# COPYRIGHT NOTICE") {
		t.Error("copyright banner was dropped; attribution must be preserved")
	}
	if !strings.Contains(got, "# banner-skill") {
		t.Error("body was dropped")
	}
}

func TestReadSkillDerivesDescriptionWhenMissing(t *testing.T) {
	root := t.TempDir()
	doc := "# Some Skill\n\n```bash\nignored\n```\n\nRuns an aligner over paired FASTQ files.\n"
	dir := writeSkill(t, root, "some-skill", doc, nil)
	src := SourceConfig{Name: "demo", Root: ".", DomainFrom: domainFromFrontMatter}

	sk, err := readSkill(dir, "some-skill", src)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != "some-skill" {
		t.Errorf("name = %q, want the directory name", sk.Name)
	}
	// Headings and fences are skipped; the first real prose line wins, so the
	// skill stays findable by search instead of showing an empty description.
	if sk.Description != "Runs an aligner over paired FASTQ files." {
		t.Errorf("description = %q", sk.Description)
	}
}

// Some upstream front matter is not valid YAML: a plain-scalar description
// containing ": " reads as a nested mapping. Strict parsing alone would lose the
// skill's real name and, worse, derive a description from the body that starts
// with a literal "name:" line — which the framework's flattening front-matter
// reader would then take as the skill's name, collapsing several skills onto one
// index key and dropping them.
func TestReadSkillRecoversFromInvalidYAMLFrontMatter(t *testing.T) {
	root := t.TempDir()
	doc := `---
name: bio-methylation-array-qc-filtering
description: Filters probes and runs sample-identity QC: getSex prediction vs sample sheet.
tool_type: r
primary_tool: minfi
---

## Version Compatibility
`
	dir := writeSkill(t, root, "methylation-analysis/array-qc-filtering", doc, nil)
	src := SourceConfig{Name: "bioskills", Root: ".", DomainFrom: domainFromPathSegment}

	sk, err := readSkill(dir, "methylation-analysis/array-qc-filtering", src)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != "bio-methylation-array-qc-filtering" {
		t.Errorf("name = %q, want the upstream front-matter name", sk.Name)
	}
	if !strings.HasPrefix(sk.Description, "Filters probes and runs sample-identity QC") {
		t.Errorf("description = %q, want the upstream description", sk.Description)
	}
	// The rewritten front matter must not contain a second "name:" anywhere, at
	// any indentation, or the framework indexes this skill under the wrong key.
	body := string(sk.Doc)
	_, after, _ := strings.Cut(body, "---\n")
	block, _, _ := strings.Cut(after, "\n---\n")
	if strings.Count(block, "name:") != 1 {
		t.Errorf("front matter has %d name: lines, want exactly 1:\n%s",
			strings.Count(block, "name:"), block)
	}
}

func TestPlainScalarStaysAValidUnquotedScalar(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// ": " would open a nested mapping; the space goes, the text stays.
		{"Runs QC: getSex prediction", "Runs QC:getSex prediction"},
		// " #" would start a comment and truncate the value.
		{"Counts reads # per cell", "Counts reads per cell"},
		// A leading indicator would change the scalar's type.
		{"[experimental] aligner wrapper", "experimental] aligner wrapper"},
		// Newlines and runs of space collapse to single spaces.
		{"line one\n  line two", "line one line two"},
		// A trailing colon would still read as a key.
		{"See also:", "See also"},
	}
	for _, tc := range cases {
		if got := plainScalar(tc.in); got != tc.want {
			t.Errorf("plainScalar(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Nested upstream metadata has to survive without being re-emitted as an
// indented block, which the framework's line-oriented reader would flatten into
// bogus top-level keys.
func TestNormalizeDocEmitsOneLinePerKey(t *testing.T) {
	root := t.TempDir()
	doc := `---
name: analyze-fasta
description: Analyse a FASTA file.
license: MIT
metadata:
  domain: genomics
  name: not-the-skill-name
  demo_data:
    - path: example_data/demo.fasta
      description: Synthetic sequence
---

# analyze-fasta
`
	dir := writeSkill(t, root, "analyze-fasta", doc, nil)
	src := SourceConfig{Name: "clawbio", Root: ".", DomainFrom: domainFromFrontMatter}

	sk, err := readSkill(dir, "analyze-fasta", src)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Domain != "genomics" {
		t.Errorf("domain = %q, want genomics", sk.Domain)
	}
	_, after, _ := strings.Cut(string(sk.Doc), "---\n")
	block, _, _ := strings.Cut(after, "\n---\n")
	for line := range strings.SplitSeq(strings.TrimSpace(block), "\n") {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			t.Errorf("front matter has an indented line, which the framework "+
				"reader flattens into a bogus key: %q\nblock:\n%s", line, block)
		}
	}
	// The nested name must not have become a top-level key.
	if strings.Contains(block, "\nname:") || !strings.HasPrefix(block, "name: analyze-fasta") {
		t.Errorf("nested name leaked to the top level:\n%s", block)
	}
	// ...but the metadata itself is still there, as a single-line value.
	if !strings.Contains(block, "not-the-skill-name") {
		t.Errorf("upstream metadata was dropped:\n%s", block)
	}
}

func TestSplitFrontMatterTerminators(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"trailing newline", "---\nname: a\n---\nbody text\n"},
		{"closes at EOF", "---\nname: a\n---"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm, _, _ := splitFrontMatter([]byte(tc.raw))
			if fm.str("name") != "a" {
				t.Errorf("name = %q, want a", fm.str("name"))
			}
		})
	}

	// A horizontal rule in the body must not be mistaken for the terminator.
	fm, body, _ := splitFrontMatter([]byte("---\nname: a\ndescription: d\n---\n\ntext\n\n-----\n\nmore\n"))
	if fm.str("description") != "d" {
		t.Errorf("description = %q, want d", fm.str("description"))
	}
	if !strings.Contains(body, "-----") {
		t.Errorf("body lost its horizontal rule: %q", body)
	}
}

func TestScanSourceAppliesDenyAndNesting(t *testing.T) {
	root := t.TempDir()
	srcRoot := filepath.Join(root, "skills")
	docFor := func(name string) string {
		return "---\nname: " + name + "\ndescription: d\n---\n\nbody\n"
	}
	writeSkill(t, srcRoot, "genomics/keeper", docFor("keeper"), nil)
	writeSkill(t, srcRoot, "genomics/thing-mcp", docFor("thing-mcp"), nil)
	writeSkill(t, srcRoot, "genomics/outer", docFor("outer"), nil)
	writeSkill(t, srcRoot, "genomics/outer/examples/demo", docFor("demo"), nil)
	writeSkill(t, srcRoot, "genomics/__pycache__/junk", docFor("junk"), nil)

	m := &Manifest{
		Include: IncludeRules{Extensions: []string{".md"}, MaxFileBytes: 1 << 20},
		Exclude: ExcludeRules{Dirs: []string{"__pycache__"}},
		Deny:    []string{"*-mcp"},
	}
	src := SourceConfig{Name: "demo", Path: ".", Root: "skills", DomainFrom: domainFromPathSegment}

	found, drops, err := scanSource(root, src, m)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(found))
	for _, sk := range found {
		names = append(names, sk.Name)
	}
	if strings.Join(names, ",") != "keeper,outer" {
		t.Errorf("kept %v, want [keeper outer]", names)
	}
	// thing-mcp by deny rule, demo as nested; junk never surfaces because
	// __pycache__ is pruned before the walk descends into it.
	if len(drops) != 2 {
		t.Fatalf("drops = %+v, want 2 (deny + nested)", drops)
	}
}

// The three bundled upstream libraries are the real input, so assert the domain
// wiring against them when the submodules are populated. This is what catches a
// manifest whose domain-from strategy silently collapses every skill into one
// bucket — the fixture tests above cover the resolution logic, but only the real
// trees prove it matches how upstream actually lays its skills out.
//
// The submodules are optional: a plain `git clone` without --recursive, which is
// what CI does for the test job, leaves them unpopulated. That must skip rather
// than fail, and "unpopulated" has to be judged by whether any skills were found
// — git creates the submodule *directory* on checkout even when the content was
// never fetched, so an existence check alone reports a false positive for any
// source whose root is the repository root.
func TestScanRealSourcesHaveDistinctDomains(t *testing.T) {
	repoRoot := "../.."
	m, err := loadManifest(filepath.Join(repoRoot, "skills", "sources.yaml"))
	if err != nil {
		t.Skipf("manifest unavailable: %v", err)
	}

	exercised := 0
	for _, src := range m.Sources {
		if !src.Redistribute {
			continue
		}
		found, _, err := scanSource(repoRoot, src, m)
		if err != nil {
			// The source root does not exist at all.
			t.Logf("skipping %s: %v", src.Name, err)
			continue
		}
		if len(found) == 0 {
			t.Logf("skipping %s: submodule directory is present but empty", src.Name)
			continue
		}
		exercised++

		domains := map[string]int{}
		for _, sk := range found {
			domains[sk.Domain]++
		}
		if len(domains) < 2 {
			t.Errorf("%s: %d skills collapsed into domains %v — "+
				"domain-from %q is not resolving",
				src.Name, len(found), domains, src.DomainFrom)
		}
	}

	if exercised == 0 {
		t.Skip("no upstream skill submodules are populated; " +
			"run `make skills-submodules` to exercise this test")
	}
}
