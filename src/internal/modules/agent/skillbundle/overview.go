package skillbundle

import (
	"fmt"
	"sort"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/skill"
)

// overviewHeader must match the framework's skills overview header so the
// processor recognises the section instead of prefixing its own.
const overviewHeader = "Available skills:"

// domainsPerLine keeps the rendered domain index compact without producing one
// unreadable 2000-character line.
const domainsPerLine = 6

// RenderOverview renders the "Available skills" block injected into the system
// prompt on every request.
//
// The framework's default renders one "- name: description" line per visible
// skill. With the built-in library that is roughly 100k tokens on every single
// request, which is both unaffordable and worse for the model than a short
// index — 700 near-identical descriptions bury the few that matter.
//
// So built-in skills are summarised as domain counts and the model is pointed at
// skill_search to resolve a name. User-uploaded skills are listed in full: there
// are only ever a handful, and they are the ones the user most expects the agent
// to know about without being asked.
func RenderOverview(catalog *Catalog, summaries []skill.Summary, searchTool string) string {
	builtin := map[string]struct{}{}
	if catalog != nil {
		for _, e := range catalog.entries {
			builtin[e.Name] = struct{}{}
		}
	}

	var user []skill.Summary
	bundled := 0
	for _, s := range summaries {
		if _, ok := builtin[s.Name]; ok {
			bundled++
			continue
		}
		user = append(user, s)
	}

	var b strings.Builder
	b.WriteString(overviewHeader + "\n")

	domains := catalog.Domains()
	if bundled > 0 && len(domains) > 0 {
		fmt.Fprintf(&b, "%d built-in skills across %d domains. They are NOT listed here — "+
			"call %s(\"<what you need>\") to get names and descriptions, then "+
			"skill_load(<name>) to load one. Never invent or guess a skill name.\n",
			bundled, len(domains), searchTool)
		b.WriteString("Domains (skill counts):\n")
		for i, d := range domains {
			if i%domainsPerLine == 0 {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, " %s(%d)", d.Domain, d.Skills)
			if i%domainsPerLine == domainsPerLine-1 || i == len(domains)-1 {
				b.WriteString("\n")
			}
		}
	}

	if len(user) > 0 {
		// Sorted so the block is byte-identical between turns, which keeps
		// the prompt prefix cacheable.
		sort.Slice(user, func(i, j int) bool { return user[i].Name < user[j].Name })
		b.WriteString("Your uploaded skills (load by name directly):\n")
		for _, s := range user {
			fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
		}
	}

	if bundled == 0 && len(user) == 0 {
		// The framework skips the whole section when no skills are visible;
		// saying so explicitly stops the model from hallucinating skill
		// calls that cannot succeed.
		b.WriteString("No skills are available. Do not call skill_load or " +
			searchTool + "; solve the task with the other tools.\n")
	}
	return b.String()
}
