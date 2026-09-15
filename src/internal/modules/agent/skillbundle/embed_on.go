//go:build skills

package skillbundle

import (
	"embed"
	"io/fs"
)

// bundleFS holds the generated skill library. Build the bundle first
// (`make skills-bundle`, which needs the submodules under skills/upstream/),
// then build with `-tags skills`. Without the tag this file is excluded and the
// binary reads the library from disk instead — see embed_off.go.
//
// The `all:` prefix is deliberately NOT used: the bundler emits no dotfiles, and
// excluding them keeps any stray editor or VCS state out of the binary.
//
//go:embed bundle
var bundleFS embed.FS

// embedded returns the baked-in bundle, or nil in a build without the tag.
func embedded() fs.FS {
	sub, err := fs.Sub(bundleFS, "bundle")
	if err != nil {
		// Unreachable: the //go:embed above fails the build if bundle/ is
		// missing, so the subtree always exists here.
		return nil
	}
	return sub
}
