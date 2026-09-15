// Package skillbundle owns the built-in skill library: the on-disk layout the
// bundler (cmd/skillsbundle) produces, the optional embedded copy baked into
// release binaries, and the skill repository the agent loads it through.
//
// The library is large — roughly 700 skills across ~70 domains, pruned from
// four upstream libraries vendored as submodules under skills/upstream/. Two
// consequences drive this package's design:
//
//   - The framework's skill.FSRepository re-parses every SKILL.md's front
//     matter on each Summaries() call, and Summaries() is called on every LLM
//     request. At this scale that is ~700 file reads per chat turn, so the
//     repository here serves summaries from the pre-built index instead and
//     only touches disk when a skill is actually loaded or staged.
//   - The framework injects every visible skill's name and description into the
//     system prompt. Verbatim that is ~100k tokens per request, so the agent
//     renders a compact domain index instead and lets the model search (see
//     Catalog.Search and RenderOverview).
package skillbundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// IndexFile is the bundle-relative name of the generated catalog. It sits at
// the bundle root alongside one directory per source.
const IndexFile = "index.json"

// NoticeFile is the bundle-relative attribution file listing every upstream
// source, its licence, and the commit the bundle was cut from.
const NoticeFile = "NOTICE"

// IndexVersion is the schema version of IndexFile. The runtime refuses an
// index it does not understand rather than silently loading a partial catalog.
const IndexVersion = 1

// Index is the generated bundle catalog, written to IndexFile by
// cmd/skillsbundle and read back at startup.
type Index struct {
	Version     int    `json:"version"`
	GeneratedBy string `json:"generated_by"`
	// Hash is a content digest over every bundled file. The extractor uses it
	// to skip re-writing an already-materialised bundle.
	Hash   string   `json:"hash"`
	Files  int      `json:"files"`
	Bytes  int64    `json:"bytes"`
	Source []Source `json:"sources"`
	Skills []Entry  `json:"skills"`
}

// Source records provenance for one upstream skill library.
type Source struct {
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	License string `json:"license,omitempty"`
	// Commit is the submodule revision the bundle was generated from.
	Commit string `json:"commit,omitempty"`
	Skills int    `json:"skills"`
}

// Entry is one bundled skill.
type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Source      string `json:"source,omitempty"`
	// Dir is the skill directory relative to the bundle root, forward-slashed.
	Dir  string   `json:"dir"`
	Tags []string `json:"tags,omitempty"`
}

// LoadIndex reads and validates the catalog at the root of a materialised
// bundle.
func LoadIndex(root string) (*Index, error) {
	raw, err := os.ReadFile(filepath.Join(root, IndexFile))
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("parse %s: %w", IndexFile, err)
	}
	if idx.Version != IndexVersion {
		return nil, fmt.Errorf("%s: unsupported index version %d (want %d) — "+
			"regenerate with `make skills-bundle`", IndexFile, idx.Version, IndexVersion)
	}
	return &idx, nil
}

// Marshal renders the index as the indented JSON written to IndexFile. Kept
// here so the generator and any test round-trip through identical formatting.
func (i *Index) Marshal() ([]byte, error) {
	out, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
