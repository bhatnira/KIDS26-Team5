package skillbundle

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// stampFile records the hash of the bundle already materialised in a directory,
// so restarts skip the copy. It lives inside the extracted tree and is not part
// of the bundle itself.
const stampFile = ".bundle-hash"

// extract materialises an embedded bundle into dest.
//
// Extraction is necessary rather than incidental: the framework stages a skill
// by copying its directory from the local filesystem into the sandbox
// (skill.Repository.Path must return a real path), so an embedded-only bundle
// could never run. It is hash-gated, so the common case — a restart against an
// already-populated volume or container filesystem — does no work.
//
// Returns true when files were written.
func extract(src fs.FS, dest string) (bool, error) {
	want, err := embeddedHash(src)
	if err != nil {
		return false, err
	}
	if current, err := readStamp(dest); err == nil && current == want {
		return false, nil
	}

	// Replace wholesale rather than merge: a skill deleted upstream must not
	// linger in the extracted tree, and a half-written directory from an
	// interrupted previous run must not be trusted.
	if err := os.RemoveAll(dest); err != nil {
		return false, fmt.Errorf("clear skill bundle dir %s: %w", dest, err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return false, err
	}

	walkErr := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out := filepath.Join(dest, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		body, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		// embed drops the executable bit, so restore it the same way the
		// bundler decides it: a shebang means the file is meant to be run.
		mode := os.FileMode(0o644)
		if strings.HasPrefix(string(body), "#!") {
			mode = 0o755
		}
		return os.WriteFile(out, body, mode)
	})
	if walkErr != nil {
		return false, fmt.Errorf("extract skill bundle to %s: %w", dest, walkErr)
	}

	// Non-secret cache marker, readable by whoever inspects the unpacked tree.
	stamp := filepath.Join(dest, stampFile)
	if err := os.WriteFile(stamp, []byte(want+"\n"), 0o644); err != nil { // #nosec G306
		return false, err
	}
	return true, nil
}

// embeddedHash reads the content hash from an embedded bundle's index.
func embeddedHash(src fs.FS) (string, error) {
	raw, err := fs.ReadFile(src, IndexFile)
	if err != nil {
		return "", fmt.Errorf("embedded bundle has no %s: %w", IndexFile, err)
	}
	var idx Index
	if err := json.Unmarshal(raw, &idx); err != nil {
		return "", fmt.Errorf("parse embedded %s: %w", IndexFile, err)
	}
	if idx.Hash == "" {
		return "", fmt.Errorf("embedded %s carries no hash", IndexFile)
	}
	return idx.Hash, nil
}

// readStamp reads the hash of the bundle already extracted into dest.
func readStamp(dest string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dest, stampFile))
	if err != nil {
		return "", err
	}
	// A stamp without its index means an interrupted extraction; treat it as
	// no stamp at all so the caller re-materialises.
	if _, err := os.Stat(filepath.Join(dest, IndexFile)); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}
