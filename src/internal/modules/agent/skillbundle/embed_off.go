//go:build !skills

package skillbundle

import "io/fs"

// embedded reports no baked-in bundle. Builds without the `skills` tag stay
// small and compile without a generated bundle on disk, which is what local
// development and `go test ./...` want; the library is then read from the
// configured on-disk root instead.
func embedded() fs.FS { return nil }
