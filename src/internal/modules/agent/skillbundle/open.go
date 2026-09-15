package skillbundle

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"trpc.group/trpc-go/trpc-agent-go/skill"
)

// DefaultDiskRoot is where cmd/skillsbundle writes the bundle, and therefore
// where a build without the `skills` tag looks for it when no root is
// configured. Relative to the repository root, which is what `make dev` and
// `go run . web` run from.
const DefaultDiskRoot = "internal/modules/agent/skillbundle/bundle"

// defaultExtractDirName is appended to the OS temp dir when an embedded bundle
// has nowhere configured to extract to.
const defaultExtractDirName = "antelope-skill-bundle"

// Options configures how the built-in library is located.
type Options struct {
	// DiskRoot is the bundle directory to read when the binary carries no
	// embedded copy (dev builds). Empty falls back to DefaultDiskRoot.
	DiskRoot string

	// ExtractRoot is where an embedded bundle is materialised. Empty falls
	// back to a directory under the OS temp dir. Set this to a persistent
	// path (a container filesystem path or a volume) so restarts skip the
	// copy.
	ExtractRoot string
}

// Library is a loaded built-in skill library. A nil *Library is valid and means
// "no built-in skills"; every method tolerates it.
type Library struct {
	root     string
	catalog  *Catalog
	repo     *Repository
	embedded bool
	// extracted reports whether this process wrote the tree, as opposed to
	// finding it already materialised.
	extracted bool
}

// Open locates, materialises if needed, and loads the built-in library.
//
// It returns (nil, nil) when there is no library to load: a build without the
// `skills` tag and no generated bundle on disk. That is a supported development
// state — the agent simply has no built-in skills — so it must not stop the
// server from starting. Anything else (a corrupt index, an unwritable extract
// directory, an embedded bundle that fails to unpack) is a real error.
// An embedded bundle always wins: a binary built to be self-contained must not
// have its skill library quietly replaced by whatever happens to sit at
// DiskRoot on the host.
func Open(opts Options) (*Library, error) {
	if src := embedded(); src != nil {
		return openEmbedded(src, opts)
	}
	return openDisk(opts.DiskRoot)
}

// openDisk loads a bundle that is already materialised on disk. Split out from
// Open so it can be exercised regardless of whether the test binary was built
// with an embedded bundle.
func openDisk(root string) (*Library, error) {
	if root == "" {
		root = DefaultDiskRoot
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(abs, IndexFile)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return load(abs, false, false)
}

// openEmbedded extracts the baked-in bundle and loads it from disk. The
// framework stages skills by copying directories out of the local filesystem,
// so the embedded copy has to land on disk before it is usable.
func openEmbedded(src fs.FS, opts Options) (*Library, error) {
	dest := opts.ExtractRoot
	if dest == "" {
		dest = filepath.Join(os.TempDir(), defaultExtractDirName)
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return nil, err
	}
	wrote, err := extract(src, abs)
	if err != nil {
		return nil, err
	}
	return load(abs, true, wrote)
}

// load reads the catalog and builds the repository over a materialised bundle.
func load(root string, isEmbedded, wrote bool) (*Library, error) {
	idx, err := LoadIndex(root)
	if err != nil {
		return nil, err
	}
	catalog := NewCatalog(idx)
	repo, err := NewRepository(root, catalog)
	if err != nil {
		return nil, err
	}
	// The index and the tree are generated together, so a mismatch means the
	// tree was edited or a copy was truncated. Fail loudly rather than serve a
	// library whose catalog promises skills that cannot be staged.
	if got, want := len(repo.inner.Summaries()), len(idx.Skills); got < want {
		return nil, fmt.Errorf("skill bundle at %s is incomplete: index lists %d skills, "+
			"%d found on disk — regenerate with `make skills-bundle`", root, want, got)
	}
	return &Library{
		root:      root,
		catalog:   catalog,
		repo:      repo,
		embedded:  isEmbedded,
		extracted: wrote,
	}, nil
}

// Root is the directory the library was loaded from.
func (l *Library) Root() string {
	if l == nil {
		return ""
	}
	return l.root
}

// Catalog exposes the skill catalog, or nil when no library is loaded.
func (l *Library) Catalog() *Catalog {
	if l == nil {
		return nil
	}
	return l.catalog
}

// Repository exposes the framework-facing repository. It returns an untyped nil
// when no library is loaded, so callers composing repositories can test it
// against nil without tripping over a typed-nil interface value.
func (l *Library) Repository() skill.Repository {
	if l == nil || l.repo == nil {
		return nil
	}
	return l.repo
}

// Len is the number of built-in skills.
func (l *Library) Len() int {
	if l == nil {
		return 0
	}
	return l.catalog.Len()
}

// Embedded reports whether the library came from the binary rather than disk.
func (l *Library) Embedded() bool {
	if l == nil {
		return false
	}
	return l.embedded
}

// Extracted reports whether this process wrote the bundle to disk, as opposed to
// reusing an already-materialised copy.
func (l *Library) Extracted() bool {
	if l == nil {
		return false
	}
	return l.extracted
}
