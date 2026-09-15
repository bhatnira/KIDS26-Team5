package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// domainFromPathSegment derives a skill's domain from the first path segment
// below the source root (bioskills / bioclaw layout: <domain>/<skill>/SKILL.md).
const domainFromPathSegment = "path-segment"

// domainFromFrontMatter reads the domain from SKILL.md front matter
// (metadata.domain, domain, or category) for sources with a flat layout.
const domainFromFrontMatter = "front-matter"

// Manifest is skills/sources.yaml: which upstream trees to bundle and which
// files and skills to keep.
type Manifest struct {
	Version int            `yaml:"version"`
	Include IncludeRules   `yaml:"include"`
	Exclude ExcludeRules   `yaml:"exclude"`
	Deny    []string       `yaml:"deny"`
	Sources []SourceConfig `yaml:"sources"`
}

// IncludeRules bounds what a skill directory may contribute to the bundle.
type IncludeRules struct {
	Extensions   []string `yaml:"extensions"`
	MaxFileBytes int64    `yaml:"max-file-bytes"`
	LicenseFiles []string `yaml:"license-files"`
}

// ExcludeRules lists directory and file basenames pruned at any depth.
type ExcludeRules struct {
	Dirs  []string `yaml:"dirs"`
	Files []string `yaml:"files"`
}

// SourceConfig describes one upstream submodule.
type SourceConfig struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
	// Root is the subdirectory of Path that holds the skill tree ("." when
	// skills sit at the repository root, "skills" for the rest).
	Root string `yaml:"root"`
	// DomainFrom selects the domain derivation strategy; see the
	// domainFrom* constants.
	DomainFrom string `yaml:"domain-from"`
	URL        string `yaml:"url"`
	License    string `yaml:"license"`
	// Redistribute gates whether this source may be baked into a
	// distributed artifact. Sources with no licence grant set it to false
	// and are skipped unless -include-unlicensed is passed.
	Redistribute bool `yaml:"redistribute"`
	// Priority breaks skill-name collisions: lowest wins.
	Priority int      `yaml:"priority"`
	Deny     []string `yaml:"deny"`
}

// loadManifest reads and validates skills/sources.yaml.
func loadManifest(file string) (*Manifest, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	if len(m.Sources) == 0 {
		return nil, fmt.Errorf("%s: no sources declared", file)
	}
	if m.Include.MaxFileBytes <= 0 {
		return nil, fmt.Errorf("%s: include.max-file-bytes must be > 0", file)
	}
	if len(m.Include.Extensions) == 0 {
		return nil, fmt.Errorf("%s: include.extensions must not be empty", file)
	}
	seen := map[string]bool{}
	for i := range m.Sources {
		s := &m.Sources[i]
		if s.Name == "" || s.Path == "" {
			return nil, fmt.Errorf("%s: source %d needs both name and path", file, i)
		}
		if seen[s.Name] {
			return nil, fmt.Errorf("%s: duplicate source name %q", file, s.Name)
		}
		seen[s.Name] = true
		if s.Root == "" {
			s.Root = "."
		}
		if s.DomainFrom == "" {
			s.DomainFrom = domainFromFrontMatter
		}
		if s.DomainFrom != domainFromPathSegment && s.DomainFrom != domainFromFrontMatter {
			return nil, fmt.Errorf("%s: source %q has unknown domain-from %q",
				file, s.Name, s.DomainFrom)
		}
	}
	// Bundle order follows priority so collision resolution is deterministic
	// regardless of how the file is written.
	sort.SliceStable(m.Sources, func(i, j int) bool {
		return m.Sources[i].Priority < m.Sources[j].Priority
	})
	return &m, nil
}

// allowedExt reports whether a file's extension is in the allowlist. Matching
// is case-insensitive so .R and .r, .MD and .md all pass.
func (r IncludeRules) allowedExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	return slices.ContainsFunc(r.Extensions, func(want string) bool {
		return strings.EqualFold(want, ext)
	})
}

// prunedDir reports whether a directory basename is excluded at any depth.
// Directories starting with a dot are always pruned: they hold VCS and tool
// state, never skill content, and //go:embed skips them anyway.
func (r ExcludeRules) prunedDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	return slices.ContainsFunc(r.Dirs, func(want string) bool {
		return strings.EqualFold(want, name)
	})
}

// prunedFile reports whether a file basename is excluded at any depth.
func (r ExcludeRules) prunedFile(name string) bool {
	return slices.ContainsFunc(r.Files, func(want string) bool {
		return strings.EqualFold(want, name)
	})
}

// denied reports whether a skill is dropped by the manifest-wide or
// source-specific deny lists. Patterns are path.Match globs tested against both
// the skill name and its slash-separated path relative to the source root, plus
// every individual segment of that path, so "benchmarks" drops
// "benchmarks/foo" without needing a trailing wildcard.
func denied(patterns []string, name, relPath string) (string, bool) {
	candidates := append([]string{name, relPath}, strings.Split(relPath, "/")...)
	for _, pat := range patterns {
		for _, c := range candidates {
			if c == "" {
				continue
			}
			if ok, err := path.Match(pat, c); err == nil && ok {
				return pat, true
			}
		}
	}
	return "", false
}
