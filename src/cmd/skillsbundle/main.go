// Command skillsbundle builds the built-in agent skill library.
//
// It reads skills/sources.yaml, walks the upstream libraries vendored as git
// submodules under skills/upstream/, and writes a pruned, de-duplicated tree
// plus a catalog to internal/modules/agent/skillbundle/bundle/. Release builds
// bake that tree into the binary via //go:embed (build tag `skills`); dev builds
// read it straight off disk.
//
// Pruning is the point: the four upstream repositories total ~380 MB of working
// tree, most of it reference datasets, sqlite databases, archives and
// screenshots. Only instructions and runnable scripts belong in a server image.
//
// Usage:
//
//	go run ./cmd/skillsbundle                      # or: make skills-bundle
//	go run ./cmd/skillsbundle -include-unlicensed   # opt into unlicensed sources
//
// Run it from the repository root, or pass -root.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"antelope/internal/modules/agent/skillbundle"

	"trpc.group/trpc-go/trpc-agent-go/skill"
)

// generatedBy is recorded in the index so a stale bundle is identifiable.
const generatedBy = "cmd/skillsbundle"

// largestSkillsReported caps the "heaviest skills" section of the report.
const largestSkillsReported = 8

// dropsListedInFull is how many dropped skills the report names individually
// before collapsing to per-reason counts. Above this, pass -v.
const dropsListedInFull = 40

// verbose forces the full drop listing regardless of dropsListedInFull.
var verbose bool

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "skillsbundle: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var (
		root       = flag.String("root", ".", "repository root")
		manifest   = flag.String("manifest", "skills/sources.yaml", "source manifest, relative to -root")
		out        = flag.String("out", filepath.Join("internal", "modules", "agent", "skillbundle", "bundle"), "bundle output directory, relative to -root")
		unlicensed = flag.Bool("include-unlicensed", false,
			"also bundle sources marked redistribute:false — these carry no redistribution grant, see skills/sources.yaml")
	)
	flag.BoolVar(&verbose, "v", false, "list every dropped skill individually")
	flag.Parse()

	repoRoot, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	m, err := loadManifest(filepath.Join(repoRoot, *manifest))
	if err != nil {
		return err
	}
	bundleRoot := filepath.Join(repoRoot, *out)

	// Collect every candidate skill first so collisions are resolved against
	// the complete picture before anything is written.
	var (
		kept     []Skill
		drops    []drop
		perSrc   = map[string]int{}
		claimed  = map[string]Skill{}
		selected []SourceConfig
	)
	for _, src := range m.Sources {
		if !src.Redistribute && !*unlicensed {
			fmt.Printf("skip   %-18s %s (redistribute: false — %s)\n",
				src.Name, src.URL, src.License)
			continue
		}
		if !src.Redistribute {
			fmt.Printf("WARNING: bundling %q, which declares no redistribution grant (%s).\n"+
				"         You are responsible for the licence exposure this creates.\n",
				src.Name, src.License)
		}
		found, srcDrops, err := scanSource(repoRoot, src, m)
		if err != nil {
			return err
		}
		drops = append(drops, srcDrops...)
		selected = append(selected, src)

		for _, sk := range found {
			if prev, dup := claimed[sk.Name]; dup {
				drops = append(drops, drop{Source: src.Name, Skill: sk.Name, Path: sk.RelPath,
					Reason: fmt.Sprintf("duplicate name, kept %s/%s", prev.Source, prev.RelPath)})
				continue
			}
			claimed[sk.Name] = sk
			kept = append(kept, sk)
			perSrc[src.Name]++
		}
	}
	if len(kept) == 0 {
		return fmt.Errorf("no skills selected — check %s and that the submodules are checked out", *manifest)
	}

	// A stale bundle must never survive a regeneration: a skill removed
	// upstream or newly denied has to disappear from the tree too.
	if err := os.RemoveAll(bundleRoot); err != nil {
		return fmt.Errorf("clear %s: %w", bundleRoot, err)
	}
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		return err
	}

	// Everything below writes through this root handle, so no upstream symlink
	// or path can place a file outside the bundle directory.
	dest, err := os.OpenRoot(bundleRoot)
	if err != nil {
		return err
	}
	defer dest.Close()

	st := &copyStats{}
	sort.Slice(kept, func(i, j int) bool { return kept[i].BundleDir() < kept[j].BundleDir() })
	for _, sk := range kept {
		if err := copySkill(dest, sk, m, st); err != nil {
			return err
		}
	}

	sources := make([]skillbundle.Source, 0, len(selected))
	for _, src := range selected {
		licenses, err := copyLicenses(dest, repoRoot, src, m, st)
		if err != nil {
			return err
		}
		if licenses == 0 && src.Redistribute {
			return fmt.Errorf("source %q declares licence %q but ships none of %v — "+
				"redistribution requires the notice to travel with the copy",
				src.Name, src.License, m.Include.LicenseFiles)
		}
		sources = append(sources, skillbundle.Source{
			Name:    src.Name,
			URL:     src.URL,
			License: src.License,
			Commit:  submoduleCommit(ctx, filepath.Join(repoRoot, src.Path)),
			Skills:  perSrc[src.Name],
		})
	}

	idx := &skillbundle.Index{
		Version:     skillbundle.IndexVersion,
		GeneratedBy: generatedBy,
		Hash:        st.hash(),
		Files:       st.Files,
		Bytes:       st.Bytes,
		Source:      sources,
		Skills:      entries(kept),
	}
	if err := writeIndex(dest, idx); err != nil {
		return err
	}
	if err := writeNotice(dest, idx); err != nil {
		return err
	}
	if err := verify(bundleRoot, idx); err != nil {
		return err
	}

	report(m, idx, st, drops)
	return nil
}

// verify re-reads the finished bundle through the framework's own repository and
// checks that every catalogued skill resolves to a directory under the name the
// catalog advertises.
//
// This is the guard against a whole class of silent breakage. The framework's
// front-matter reader flattens nesting and keys its index by the parsed name, so
// a normalisation slip can make two skills collide on one name and vanish from
// the index — leaving a catalog that advertises skills the agent can find but
// never load. Failing the build here is much cheaper than discovering it in a
// chat.
func verify(bundleRoot string, idx *skillbundle.Index) error {
	repo, err := skill.NewFSRepository(bundleRoot)
	if err != nil {
		return fmt.Errorf("verify bundle: %w", err)
	}
	var missing []string
	for _, e := range idx.Skills {
		if _, err := repo.Path(e.Name); err != nil {
			missing = append(missing, e.Name+" ("+e.Dir+")")
		}
	}
	if len(missing) > 0 {
		shown := missing
		if len(shown) > 10 {
			shown = shown[:10]
		}
		return fmt.Errorf("verify bundle: %d of %d catalogued skills are not "+
			"resolvable by the framework's skill repository — most likely a "+
			"front-matter normalisation problem causing name collisions.\n  %s",
			len(missing), len(idx.Skills), strings.Join(shown, "\n  "))
	}
	if got := len(repo.Summaries()); got != len(idx.Skills) {
		return fmt.Errorf("verify bundle: repository indexed %d skills but the catalog "+
			"lists %d; the bundle contains SKILL.md files the catalog does not know about",
			got, len(idx.Skills))
	}
	return nil
}

// entries converts the kept skills into catalog entries, sorted by name so the
// index is stable across runs.
func entries(kept []Skill) []skillbundle.Entry {
	out := make([]skillbundle.Entry, 0, len(kept))
	for _, sk := range kept {
		out = append(out, skillbundle.Entry{
			Name:        sk.Name,
			Description: sk.Description,
			Domain:      sk.Domain,
			Source:      sk.Source,
			Dir:         sk.BundleDir(),
			Tags:        sk.Tags,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// writeIndex writes the catalog. It is deliberately written after every skill
// so a partial bundle has no index and the runtime treats it as absent rather
// than as a complete library with missing skills.
func writeIndex(dest *os.Root, idx *skillbundle.Index) error {
	body, err := idx.Marshal()
	if err != nil {
		return err
	}
	// Non-secret generated content; see writeFile.
	return dest.WriteFile(skillbundle.IndexFile, body, 0o644) // #nosec G306
}

// writeNotice emits the attribution file that travels with the bundle.
func writeNotice(dest *os.Root, idx *skillbundle.Index) error {
	var b strings.Builder
	b.WriteString("Antelope built-in agent skill library\n")
	b.WriteString("====================================\n\n")
	b.WriteString("This directory is generated by " + generatedBy + " from the upstream\n")
	b.WriteString("libraries listed below, each vendored as a git submodule under\n")
	b.WriteString("skills/upstream/. Skill content is redistributed under its own licence;\n")
	b.WriteString("each source's licence text is kept alongside its skills.\n\n")
	for _, s := range idx.Source {
		fmt.Fprintf(&b, "%s\n", s.Name)
		fmt.Fprintf(&b, "  upstream: %s\n", s.URL)
		fmt.Fprintf(&b, "  licence:  %s\n", s.License)
		if s.Commit != "" {
			fmt.Fprintf(&b, "  commit:   %s\n", s.Commit)
		}
		fmt.Fprintf(&b, "  skills:   %d\n", s.Skills)
		if strings.EqualFold(s.License, "UNLICENSED") {
			b.WriteString("  NOTE:     this source declares no redistribution grant; it was\n")
			b.WriteString("            bundled via -include-unlicensed.\n")
		}
		b.WriteString("\n")
	}
	// Non-secret generated content; see writeFile.
	return dest.WriteFile(skillbundle.NoticeFile, []byte(b.String()), 0o644) // #nosec G306
}

// submoduleCommit records which upstream revision a source was cut from. Best
// effort: provenance metadata is not worth failing a build over.
func submoduleCommit(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// report prints the human-facing build summary: what shipped, what was dropped
// and why, and where the remaining weight sits.
func report(m *Manifest, idx *skillbundle.Index, st *copyStats, drops []drop) {
	fmt.Printf("\nbundle: %d skills, %d files, %s\n",
		len(idx.Skills), idx.Files, human(idx.Bytes))
	for _, s := range idx.Source {
		fmt.Printf("  %-18s %4d skills  %-12s %s\n", s.Name, s.Skills, s.License, shortCommit(s.Commit))
	}

	domains := map[string]int{}
	for _, e := range idx.Skills {
		domains[e.Domain]++
	}
	fmt.Printf("  %d domains\n", len(domains))

	fmt.Printf("\npruned: %d files by extension, %d over %s, %s of upstream weight left out\n",
		st.SkippedByExt, st.SkippedBySize, human(m.Include.MaxFileBytes), human(st.SkippedBytes))

	if len(drops) > 0 {
		byReason := map[string]int{}
		for _, d := range drops {
			byReason[reasonClass(d.Reason)]++
		}
		fmt.Printf("\ndropped %d skills:\n", len(drops))
		for _, k := range sortedKeys(byReason) {
			fmt.Printf("  %-24s %d\n", k, byReason[k])
		}
		// Name every drop while the list is short enough to read. Silently
		// swallowing exclusions is how a bundle ends up missing a skill
		// nobody can explain.
		if verbose || len(drops) <= dropsListedInFull {
			fmt.Println()
			for _, d := range drops {
				label := d.Skill
				if label == "" {
					label = d.Path
				}
				fmt.Printf("  - %-44s %-18s %s\n", label, d.Source, d.Reason)
			}
		} else {
			fmt.Printf("  (%d drops; pass -v to list them)\n", len(drops))
		}
	}

	fmt.Printf("\nheaviest skills:\n")
	for _, s := range st.topLargest(largestSkillsReported) {
		fmt.Printf("  %-52s %s\n", s.Skill, human(s.Bytes))
	}
	fmt.Printf("\nhash: %s\n", idx.Hash[:16])
}

// reasonClass collapses per-skill drop reasons into reportable buckets.
func reasonClass(reason string) string {
	switch {
	case strings.HasPrefix(reason, "duplicate name"):
		return "duplicate name"
	case strings.HasPrefix(reason, "nested inside"):
		return "nested skill"
	case strings.HasPrefix(reason, "source deny rule"):
		return "source deny rule"
	case strings.HasPrefix(reason, "deny rule"):
		return "deny rule"
	default:
		return "unreadable"
	}
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

// human renders a byte count for the build log.
func human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB"}
	v := float64(n)
	for _, u := range units {
		v /= unit
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, u)
		}
	}
	return fmt.Sprintf("%.1f TiB", v)
}
