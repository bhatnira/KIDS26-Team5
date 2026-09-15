package skillbundle

import (
	"cmp"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// Catalog is the in-memory view of a bundle's index: every built-in skill,
// grouped by domain and searchable by keyword. It is immutable after Open and
// safe for concurrent use.
type Catalog struct {
	entries []Entry
	byName  map[string]Entry
	// domains holds skill counts per domain, sorted by count then name so the
	// compact overview lists the biggest domains first.
	domains []DomainCount
	sources []Source
	// tokens caches each entry's searchable terms, so Search does not
	// re-tokenise 700 descriptions on every call.
	tokens []map[string]struct{}
}

// DomainCount is one domain and how many skills it holds.
type DomainCount struct {
	Domain string
	Skills int
}

// Match is one search hit.
type Match struct {
	Entry Entry
	Score int
}

// NewCatalog builds a catalog from a generated index.
func NewCatalog(idx *Index) *Catalog {
	if idx == nil {
		return &Catalog{byName: map[string]Entry{}}
	}
	c := &Catalog{
		entries: slices.Clone(idx.Skills),
		byName:  make(map[string]Entry, len(idx.Skills)),
		sources: slices.Clone(idx.Source),
	}
	counts := map[string]int{}
	for _, e := range c.entries {
		// The index is generated with unique names, but a hand-edited or
		// half-written index must not silently shadow: first wins, matching
		// the framework's own repository semantics.
		if _, dup := c.byName[e.Name]; !dup {
			c.byName[e.Name] = e
		}
		counts[e.Domain]++
	}
	for domain, n := range counts {
		c.domains = append(c.domains, DomainCount{Domain: domain, Skills: n})
	}
	sort.Slice(c.domains, func(i, j int) bool {
		if c.domains[i].Skills != c.domains[j].Skills {
			return c.domains[i].Skills > c.domains[j].Skills
		}
		return c.domains[i].Domain < c.domains[j].Domain
	})
	c.tokens = make([]map[string]struct{}, len(c.entries))
	for i, e := range c.entries {
		c.tokens[i] = tokenize(e.Name, e.Domain, e.Description, strings.Join(e.Tags, " "))
	}
	return c
}

// Len is the number of skills in the catalog.
func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.entries)
}

// Entries returns every catalogued skill, sorted by name.
func (c *Catalog) Entries() []Entry {
	if c == nil {
		return nil
	}
	return slices.Clone(c.entries)
}

// Domains returns the per-domain skill counts, largest domain first.
func (c *Catalog) Domains() []DomainCount {
	if c == nil {
		return nil
	}
	return slices.Clone(c.domains)
}

// Sources returns the upstream provenance records.
func (c *Catalog) Sources() []Source {
	if c == nil {
		return nil
	}
	return slices.Clone(c.sources)
}

// Lookup returns a skill by exact name.
func (c *Catalog) Lookup(name string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	e, ok := c.byName[name]
	return e, ok
}

// Search ranks skills against a free-text query and returns at most limit hits.
//
// This is the model's entry point into the library: the system prompt only
// carries domain counts, so search is how a skill name is discovered. Scoring
// deliberately favours name and domain hits over description hits, since a
// description mentioning "BAM" in passing is a much weaker signal than a skill
// actually named for it.
func (c *Catalog) Search(query string, limit int) []Match {
	if c == nil || limit <= 0 {
		return nil
	}
	terms := tokenList(query)
	if len(terms) == 0 {
		return nil
	}

	var hits []Match
	for i, e := range c.entries {
		score := 0
		matched := 0
		for _, term := range terms {
			var termScore int
			switch {
			case strings.EqualFold(e.Name, term):
				termScore = 40
			case containsFold(e.Name, term):
				termScore = 12
			case containsFold(e.Domain, term):
				termScore = 8
			}
			if _, ok := c.tokens[i][term]; ok {
				termScore += 4
			}
			if termScore == 0 {
				// Prefix matching catches the singular/plural and
				// stem variants that a bio query is full of
				// ("variant" vs "variants", "align" vs "alignment").
				for tok := range c.tokens[i] {
					if len(term) >= 4 && strings.HasPrefix(tok, term) {
						termScore = 2
						break
					}
				}
			}
			if termScore > 0 {
				matched++
				score += termScore
			}
		}
		if matched == 0 {
			continue
		}
		// Reward hitting more of the query: a skill matching every term
		// beats one matching a single common word many times over.
		score += matched * 6
		if matched == len(terms) && len(terms) > 1 {
			score += 10
		}
		hits = append(hits, Match{Entry: e, Score: score})
	}

	slices.SortFunc(hits, func(a, b Match) int {
		if n := cmp.Compare(b.Score, a.Score); n != 0 {
			return n
		}
		// Stable tie-break so identical queries always return identical
		// results, which keeps prompt caching effective.
		return cmp.Compare(a.Entry.Name, b.Entry.Name)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// InDomain returns every skill in a domain, sorted by name.
func (c *Catalog) InDomain(domain string) []Entry {
	if c == nil {
		return nil
	}
	var out []Entry
	for _, e := range c.entries {
		if strings.EqualFold(e.Domain, domain) {
			out = append(out, e)
		}
	}
	return out
}

// tokenize splits the given texts into a set of lowercase search terms.
func tokenize(texts ...string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, text := range texts {
		for _, tok := range tokenList(text) {
			out[tok] = struct{}{}
		}
	}
	return out
}

// tokenList splits text into lowercase terms, breaking on anything that is not
// a letter or digit so "single-cell", "RNA-seq" and "scRNA_seq" all decompose.
func tokenList(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 2 || stopWords[f] {
			continue
		}
		out = append(out, f)
	}
	return out
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// stopWords are terms too common in skill descriptions to carry signal. Kept
// small on purpose — domain words like "gene" or "cell" are meaningful here even
// though they are frequent.
var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "in": true, "into": true,
	"is": true, "it": true, "of": true, "on": true, "or": true, "that": true,
	"the": true, "then": true, "this": true, "to": true, "use": true,
	"using": true, "when": true, "with": true, "you": true, "your": true,
}
