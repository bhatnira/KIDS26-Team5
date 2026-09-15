// Package templates embeds the application's email templates into the binary so
// it is self-contained and does not depend on a templates/ directory being
// present on disk next to the executable at runtime.
package templates

import "embed"

// FS holds every file under this directory's mail/ subtree. Names are rooted
// here, e.g. "mail/user/auth/verification_code.tmpl".
//
//go:embed all:mail
var FS embed.FS
