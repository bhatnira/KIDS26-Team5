package agent

import (
	"testing"

	"antelope/models"
)

// uptr returns a *uint for table-driven cases.
func uptr(u uint) *uint { return &u }

// TestMCPConfigViewMasksForeignHeaders verifies that auth-bearing header values
// are only returned in the clear for rows the caller owns. Global rows
// (UserID == nil, admin-managed) and other users' rows must have their header
// values masked, since GET /agent/mcp-configs lists global rows to every
// authenticated user.
func TestMCPConfigViewMasksForeignHeaders(t *testing.T) {
	const caller = uint(42)
	const secret = "Bearer super-secret-token"
	headersJSON := `{"Authorization":"` + secret + `","X-Api-Key":"abc123"}`

	cases := []struct {
		name       string
		ownerID    *uint
		wantEditbl bool
		wantGlobal bool
		wantMasked bool
	}{
		{name: "owned by caller", ownerID: uptr(caller), wantEditbl: true, wantGlobal: false, wantMasked: false},
		{name: "global (nil owner)", ownerID: nil, wantEditbl: false, wantGlobal: true, wantMasked: true},
		{name: "owned by another user", ownerID: uptr(caller + 1), wantEditbl: false, wantGlobal: false, wantMasked: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := models.MCPConfig{UserID: tc.ownerID, HeadersJSON: headersJSON}
			view := mcpConfigView(cfg, caller)

			if got := view["editable"].(bool); got != tc.wantEditbl {
				t.Errorf("editable = %v, want %v", got, tc.wantEditbl)
			}
			if got := view["is_global"].(bool); got != tc.wantGlobal {
				t.Errorf("is_global = %v, want %v", got, tc.wantGlobal)
			}

			headers, _ := view["headers"].(map[string]string)
			if len(headers) != 2 {
				t.Fatalf("expected 2 header keys preserved, got %v", headers)
			}
			gotAuth := headers["Authorization"]
			if tc.wantMasked {
				if gotAuth == secret {
					t.Errorf("Authorization value leaked for non-owned row: %q", gotAuth)
				}
				if gotAuth != "***" {
					t.Errorf("Authorization value = %q, want masked %q", gotAuth, "***")
				}
			} else if gotAuth != secret {
				t.Errorf("Authorization value = %q, want clear %q for owned row", gotAuth, secret)
			}
		})
	}
}

// TestMCPConfigViewEmptyHeaders confirms an absent/empty header map yields a nil
// headers field rather than an empty object, for both owned and global rows.
func TestMCPConfigViewEmptyHeaders(t *testing.T) {
	const caller = uint(7)
	for _, owner := range []*uint{uptr(caller), nil} {
		cfg := models.MCPConfig{UserID: owner}
		view := mcpConfigView(cfg, caller)
		if h := view["headers"]; h != nil {
			if m, ok := h.(map[string]string); !ok || len(m) != 0 {
				t.Errorf("owner=%v: headers = %v, want nil/empty", owner, h)
			}
		}
	}
}
