package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"antelope/internal/modules/agent/skillbundle"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// SkillSearchToolName is the tool the model uses to find a built-in skill. The
// name is referenced by the system prompt and by the rendered skill overview, so
// it lives in one place.
const SkillSearchToolName = "skill_search"

const (
	// skillSearchDefaultLimit is how many hits are returned when the model
	// does not ask for a specific number. Enough to choose from, small enough
	// that a wrong guess costs little context.
	skillSearchDefaultLimit = 8
	// skillSearchMaxLimit bounds a single response so one call cannot pull the
	// whole catalog into the prompt.
	skillSearchMaxLimit = 25
)

// skillSearchTool exposes the built-in skill catalog as a searchable tool.
//
// It exists because the library is far too large to enumerate in the system
// prompt: ~700 skills of names and descriptions is roughly 100k tokens per
// request. Instead the prompt carries only domain counts (see
// skillbundle.RenderOverview) and the model resolves names through this tool.
//
// Only built-in skills are searchable. A user's own uploaded skills are listed
// in full in the overview, so there is nothing to discover for them.
type skillSearchTool struct {
	catalog *skillbundle.Catalog
}

// NewSkillSearchTool returns the skill_search tool over the given catalog.
// Returns nil when there is no catalog, so the tool is simply not registered
// rather than being offered and always failing.
func NewSkillSearchTool(catalog *skillbundle.Catalog) tool.CallableTool {
	if catalog == nil || catalog.Len() == 0 {
		return nil
	}
	return &skillSearchTool{catalog: catalog}
}

type skillSearchInput struct {
	Query  string `json:"query"`
	Domain string `json:"domain,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type skillSearchHit struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Domain      string `json:"domain,omitempty"`
}

type skillSearchOutput struct {
	Hits []skillSearchHit `json:"hits"`
	// Note carries operator-facing guidance (no matches, truncation, a bad
	// domain) so the model can correct its next call instead of guessing.
	Note string `json:"note,omitempty"`
}

func (t *skillSearchTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name: SkillSearchToolName,
		Description: fmt.Sprintf(
			"Search the %d built-in bioinformatics skills by keyword and return "+
				"matching names with descriptions. The system prompt lists only "+
				"domain names and counts, so this is how you find a skill to load — "+
				"call it with the analysis you need (\"call somatic variants from a "+
				"tumour BAM\", \"single-cell QC\", \"GWAS fine-mapping\"), then pass "+
				"the winning name to skill_load. Never guess a skill name; if search "+
				"returns nothing, say so or solve the task another way. Your own "+
				"uploaded skills are already listed in the prompt and are not "+
				"searchable here.",
			t.catalog.Len()),
		InputSchema: &tool.Schema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]*tool.Schema{
				"query": {
					Type: "string",
					Description: "What you need to do, in a few words. Free text — " +
						"tool names, file formats, assay types and analysis steps all work.",
				},
				"domain": {
					Type: "string",
					Description: "Optional: restrict results to one domain from the " +
						"prompt's domain list.",
				},
				"limit": {
					Type: "integer",
					Description: fmt.Sprintf("Optional: how many hits to return (default %d, max %d).",
						skillSearchDefaultLimit, skillSearchMaxLimit),
				},
			},
		},
		OutputSchema: &tool.Schema{
			Type: "object",
			Properties: map[string]*tool.Schema{
				"hits": {
					Type: "array",
					Items: &tool.Schema{
						Type: "object",
						Properties: map[string]*tool.Schema{
							"name":        {Type: "string"},
							"description": {Type: "string"},
							"domain":      {Type: "string"},
						},
					},
				},
				"note": {Type: "string"},
			},
		},
	}
}

func (t *skillSearchTool) Call(_ context.Context, args []byte) (any, error) {
	var in skillSearchInput
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	in.Query = strings.TrimSpace(in.Query)
	in.Domain = strings.TrimSpace(in.Domain)
	if in.Query == "" && in.Domain == "" {
		return nil, errors.New("query is required (or pass a domain to list it)")
	}

	limit := in.Limit
	if limit <= 0 {
		limit = skillSearchDefaultLimit
	}
	if limit > skillSearchMaxLimit {
		limit = skillSearchMaxLimit
	}

	// A domain with no query lists that domain; a domain with a query filters
	// the ranked results down to it.
	var (
		out      skillSearchOutput
		entries  []skillbundle.Entry
		inDomain []skillbundle.Entry
	)
	if in.Domain != "" {
		inDomain = t.catalog.InDomain(in.Domain)
		if len(inDomain) == 0 {
			return skillSearchOutput{Note: fmt.Sprintf(
				"No domain %q. Use one of the domain names listed in the prompt, "+
					"or drop the domain and search by keyword instead.", in.Domain)}, nil
		}
	}

	if in.Query == "" {
		entries = inDomain
	} else {
		keep := map[string]struct{}{}
		for _, e := range inDomain {
			keep[e.Name] = struct{}{}
		}
		// Search wider than the limit before filtering, so a domain filter
		// cannot starve the result set.
		for _, hit := range t.catalog.Search(in.Query, limit*len(t.catalog.Domains())+limit) {
			if in.Domain != "" {
				if _, ok := keep[hit.Entry.Name]; !ok {
					continue
				}
			}
			entries = append(entries, hit.Entry)
		}
	}

	total := len(entries)
	if total == 0 {
		out.Note = fmt.Sprintf("No built-in skill matches %q. Try broader or "+
			"different wording (an assay name, a file format, a tool name), or "+
			"proceed without a skill.", in.Query)
		return out, nil
	}
	if total > limit {
		entries = entries[:limit]
		out.Note = fmt.Sprintf("Showing %d of %d matches; refine the query or raise limit for more.",
			limit, total)
	}
	out.Hits = make([]skillSearchHit, 0, len(entries))
	for _, e := range entries {
		out.Hits = append(out.Hits, skillSearchHit{
			Name:        e.Name,
			Description: e.Description,
			Domain:      e.Domain,
		})
	}
	return out, nil
}
