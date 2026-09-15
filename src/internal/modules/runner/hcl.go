package runner

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RenderHCL prepends a `locals { ... }` block containing the given vars to
// the template. The template body then references them as `local.<name>`,
// and HCL's native evaluator — running inside Nomad's ParseHCLOpts — does
// the substitution with full type safety.
//
// This replaces Jinja-style string templating (gonja), which conflicts
// with HCL's own ${...} interpolation syntax and requires awkward
// {% raw %} escaping.
//
// Values are JSON-marshaled. Because HCL literal syntax is a superset of
// JSON for strings/numbers/bools/arrays/objects, the JSON representation
// is also valid HCL. Examples:
//
//	vars := map[string]interface{}{
//	    "job_id":      "nextflow-generic",
//	    "datacenters": []string{"cab", "dc1"},
//	    "cores":       2,
//	    "memory":      8000,
//	}
//
// Produces:
//
//	locals {
//	  cores       = 2
//	  datacenters = ["cab","dc1"]
//	  job_id      = "nextflow-generic"
//	  memory      = 8000
//	}
//
// Keys are sorted so the output is deterministic (useful for tests and
// caching).
func RenderHCL(template string, vars map[string]any) (string, error) {
	if len(vars) == 0 {
		return template, nil
	}

	keys := make([]string, 0, len(vars))
	for k := range vars {
		if !isValidHCLIdent(k) {
			return "", fmt.Errorf("runner: invalid HCL identifier %q", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("locals {\n")
	for _, k := range keys {
		raw, err := json.Marshal(vars[k])
		if err != nil {
			return "", fmt.Errorf("runner: marshal var %q: %w", k, err)
		}
		fmt.Fprintf(&b, "  %s = %s\n", k, raw)
	}
	b.WriteString("}\n\n")
	b.WriteString(template)
	return b.String(), nil
}

// isValidHCLIdent matches HCL's identifier grammar:
// letter (letter | digit | _ | -)*.
// We disallow '-' here because we render `local.<k>` and unquoted identifiers
// with hyphens are awkward; stick to [A-Za-z_][A-Za-z0-9_]*.
func isValidHCLIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}
