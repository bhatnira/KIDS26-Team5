package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// digestSep separates fields in the hashed file listing. A byte that cannot
// appear in a path keeps the digest unambiguous.
const digestSep = "\x00"

// copyStats accumulates what a copy pass wrote and skipped.
type copyStats struct {
	Files int
	Bytes int64
	// SkippedByExt and SkippedBySize count dropped files by rule, and
	// SkippedBytes is how much upstream weight never entered the bundle.
	SkippedByExt  int
	SkippedBySize int
	SkippedBytes  int64
	// Largest tracks the biggest bundled skills so an unexpectedly heavy
	// skill can be denied instead of silently inflating the image.
	Largest []skillSize
	// digest collects "<path>\x00<sha256>" lines for the bundle hash.
	digest []string
}

type skillSize struct {
	Skill string
	Bytes int64
}

// copySkill writes one skill directory into the bundle, keeping only files the
// manifest allows. The normalised SKILL.md replaces the upstream one so front
// matter is always first and always carries a name and description.
//
// Both sides are accessed through os.Root handles. The inputs are third-party
// repositories, so a symlink pointing out of a skill directory is a realistic
// way for content we never reviewed to end up inside a published image; a root
// handle makes that escape an error instead of a copy.
func copySkill(dest *os.Root, sk Skill, m *Manifest, st *copyStats) error {
	src, err := os.OpenRoot(sk.AbsDir)
	if err != nil {
		return fmt.Errorf("copy skill %q: %w", sk.Name, err)
	}
	defer src.Close()

	destDir := sk.BundleDir()
	if err := dest.MkdirAll(filepath.FromSlash(destDir), 0o755); err != nil {
		return err
	}

	var skillBytes int64
	walkErr := fs.WalkDir(src.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != "." && m.Exclude.prunedDir(name) {
				return fs.SkipDir
			}
			if p != "." {
				return dest.MkdirAll(filepath.FromSlash(path.Join(destDir, p)), 0o755)
			}
			return nil
		}
		if m.Exclude.prunedFile(name) {
			return nil
		}

		write := func(body []byte) error {
			if err := writeFile(dest, path.Join(destDir, p), body); err != nil {
				return err
			}
			st.record(destDir+"/"+p, body)
			skillBytes += int64(len(body))
			return nil
		}

		// SKILL.md is written from the normalised copy, never verbatim.
		if p == skillFile {
			return write(sk.Doc)
		}

		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		if !m.Include.allowedExt(name) {
			st.SkippedByExt++
			st.SkippedBytes += info.Size()
			return nil
		}
		if info.Size() > m.Include.MaxFileBytes {
			st.SkippedBySize++
			st.SkippedBytes += info.Size()
			return nil
		}

		body, readErr := src.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		return write(body)
	})
	if walkErr != nil {
		return fmt.Errorf("copy skill %q: %w", sk.Name, walkErr)
	}
	st.Largest = append(st.Largest, skillSize{Skill: sk.Name, Bytes: skillBytes})
	return nil
}

// BundleDir is the skill's directory relative to the bundle root. Sources are
// kept in separate subtrees so provenance stays visible on disk and two sources
// can ship same-named directories without clashing.
func (s Skill) BundleDir() string {
	return s.Source + "/" + s.RelPath
}

// copyLicenses copies a source's licence and attribution files into
// bundle/<source>/. MIT redistribution requires the notice to travel with the
// copy, so a source that declares a licence must actually carry one.
func copyLicenses(dest *os.Root, repoRoot string, src SourceConfig, m *Manifest, st *copyStats) (int, error) {
	if err := dest.MkdirAll(src.Name, 0o755); err != nil {
		return 0, err
	}
	found := 0
	// Licences live at the repository root, which is not necessarily the
	// skill root, so look in both.
	dirs := []string{filepath.Join(repoRoot, src.Path)}
	if src.Root != "." {
		dirs = append(dirs, filepath.Join(repoRoot, src.Path, src.Root))
	}
	for _, dir := range dirs {
		srcRoot, err := os.OpenRoot(dir)
		if err != nil {
			continue
		}
		for _, candidate := range m.Include.LicenseFiles {
			info, err := srcRoot.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
			body, err := srcRoot.ReadFile(candidate)
			if err != nil {
				srcRoot.Close()
				return found, err
			}
			rel := src.Name + "/" + candidate
			if err := writeFile(dest, rel, body); err != nil {
				srcRoot.Close()
				return found, err
			}
			st.record(rel, body)
			found++
		}
		srcRoot.Close()
	}
	return found, nil
}

// writeFile writes a bundled file, marking anything with a shebang executable.
// Mode bits do not survive //go:embed, so the same rule is applied by the
// runtime extractor — keeping the on-disk and extracted bundles identical.
//
// name is slash-separated and relative to the bundle root.
func writeFile(dest *os.Root, name string, body []byte) error {
	if dir := path.Dir(name); dir != "." {
		if err := dest.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
			return err
		}
	}
	// Bundled skills are non-secret content that has to be readable by
	// whichever user the server runs as, which is not necessarily the user that
	// generated the bundle.
	mode := os.FileMode(0o644) // #nosec G306
	if strings.HasPrefix(string(body), "#!") {
		mode = 0o755 // #nosec G302 -- a shebang script must be executable
	}
	return dest.WriteFile(filepath.FromSlash(name), body, mode)
}

// record adds a bundled file to the digest and the running totals.
func (st *copyStats) record(rel string, body []byte) {
	sum := sha256.Sum256(body)
	st.digest = append(st.digest, rel+digestSep+hex.EncodeToString(sum[:]))
	st.Files++
	st.Bytes += int64(len(body))
}

// hash finalises the content digest over every bundled file. Sorting first
// makes it independent of walk order, so an unchanged bundle keeps its hash and
// the runtime extractor can skip re-materialising it.
func (st *copyStats) hash() string {
	sort.Strings(st.digest)
	h := sha256.New()
	for _, line := range st.digest {
		h.Write([]byte(line))
		h.Write([]byte("\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// topLargest returns the n heaviest bundled skills, biggest first.
func (st *copyStats) topLargest(n int) []skillSize {
	out := append([]skillSize(nil), st.Largest...)
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
