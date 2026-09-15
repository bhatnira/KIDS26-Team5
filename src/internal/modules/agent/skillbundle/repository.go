package skillbundle

import (
	"fmt"
	"path/filepath"
	"slices"

	"trpc.group/trpc-go/trpc-agent-go/skill"
)

// Repository serves the built-in library to the framework.
//
// It answers Summaries() from the pre-built catalog and delegates Get()/Path()
// — the calls that genuinely need file contents — to an inner FSRepository.
// That split matters: Summaries() runs on every LLM request, and letting
// FSRepository serve it would re-read and re-parse ~700 SKILL.md files per chat
// turn.
type Repository struct {
	inner   *skill.FSRepository
	catalog *Catalog
	root    string
}

// Compile-time proof that Repository satisfies both the required interface and
// the optional one the framework uses to render directory locators in prompts.
var (
	_ skill.Repository       = (*Repository)(nil)
	_ skill.RootedRepository = (*Repository)(nil)
)

// NewRepository builds a repository over a materialised bundle directory.
func NewRepository(root string, catalog *Catalog) (*Repository, error) {
	inner, err := skill.NewFSRepository(root)
	if err != nil {
		return nil, fmt.Errorf("skill bundle repository at %s: %w", root, err)
	}
	return &Repository{inner: inner, catalog: catalog, root: root}, nil
}

// Summaries implements skill.Repository from the cached catalog.
func (r *Repository) Summaries() []skill.Summary {
	if r == nil || r.catalog == nil {
		return nil
	}
	entries := r.catalog.entries
	out := make([]skill.Summary, 0, len(entries))
	for _, e := range entries {
		out = append(out, skill.Summary{Name: e.Name, Description: e.Description})
	}
	return out
}

// Get implements skill.Repository, reading SKILL.md and its docs from disk.
func (r *Repository) Get(name string) (*skill.Skill, error) {
	if r == nil || r.inner == nil {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	return r.inner.Get(name)
}

// Path implements skill.Repository. The framework stages this directory into
// the sandbox when a skill runs, so it must be a real directory on disk.
func (r *Repository) Path(name string) (string, error) {
	if r == nil || r.inner == nil {
		return "", fmt.Errorf("skill %q not found", name)
	}
	return r.inner.Path(name)
}

// Roots implements skill.RootedRepository.
func (r *Repository) Roots() []string {
	if r == nil {
		return nil
	}
	return []string{r.root}
}

// Catalog exposes the catalog behind this repository.
func (r *Repository) Catalog() *Catalog {
	if r == nil {
		return nil
	}
	return r.catalog
}

// Multi presents several repositories as one, first match winning.
//
// Antelope composes the shared built-in bundle (built once at startup) with a
// per-user repository over that user's uploaded skills (built per request).
// Order is precedence order.
type Multi struct {
	repos []skill.Repository
}

var (
	_ skill.Repository       = (*Multi)(nil)
	_ skill.RootedRepository = (*Multi)(nil)
)

// NewMulti composes repositories in precedence order, ignoring nils. It returns
// the single repository unchanged when there is nothing to compose, and nil when
// there is nothing at all.
func NewMulti(repos ...skill.Repository) skill.Repository {
	kept := make([]skill.Repository, 0, len(repos))
	for _, r := range repos {
		if r != nil {
			kept = append(kept, r)
		}
	}
	switch len(kept) {
	case 0:
		return nil
	case 1:
		return kept[0]
	default:
		return &Multi{repos: kept}
	}
}

// Summaries concatenates the members' summaries, dropping later duplicates so a
// user skill cannot shadow — or be shadowed twice by — a built-in of the same
// name.
func (m *Multi) Summaries() []skill.Summary {
	if m == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []skill.Summary
	for _, r := range m.repos {
		for _, s := range r.Summaries() {
			if _, dup := seen[s.Name]; dup {
				continue
			}
			seen[s.Name] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// Get returns the first member that knows the skill.
func (m *Multi) Get(name string) (*skill.Skill, error) {
	if m == nil {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	var firstErr error
	for _, r := range m.repos {
		sk, err := r.Get(name)
		if err == nil {
			return sk, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

// Path returns the first member's directory for the skill.
func (m *Multi) Path(name string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("skill %q not found", name)
	}
	var firstErr error
	for _, r := range m.repos {
		p, err := r.Path(name)
		if err == nil {
			return p, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return "", firstErr
}

// Roots collects the roots of every member that exposes them.
func (m *Multi) Roots() []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, r := range m.repos {
		if rooted, ok := r.(skill.RootedRepository); ok {
			out = append(out, rooted.Roots()...)
		}
	}
	return slices.Compact(out)
}

// UserRepository builds a repository over a user's own skill directories.
// Returns nil when none of them exist yet, which is the common case.
func UserRepository(dirs ...string) (skill.Repository, error) {
	kept := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d == "" {
			continue
		}
		abs, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		kept = append(kept, abs)
	}
	if len(kept) == 0 {
		return nil, nil
	}
	repo, err := skill.NewFSRepository(kept...)
	if err != nil {
		return nil, err
	}
	if len(repo.Summaries()) == 0 {
		return nil, nil
	}
	return repo, nil
}
