package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillFile is the marker that makes a directory a skill. It matches the
// framework's skill.SkillFile so the bundle and the runtime agree.
const skillFile = "SKILL.md"

// maxDerivedDescription bounds a description synthesised from the Markdown body
// when front matter carries none. Long enough to be useful in search results,
// short enough not to bloat the catalog.
const maxDerivedDescription = 320

// Skill is one discovered skill directory.
type Skill struct {
	Name        string
	Description string
	Domain      string
	Tags        []string
	Source      string
	// RelPath is the skill directory relative to the source root, using
	// forward slashes.
	RelPath string
	// AbsDir is the skill directory on disk.
	AbsDir string
	// Doc is the SKILL.md contents, normalised so front matter comes first.
	Doc []byte
}

// drop records a skill that was discovered but not bundled.
type drop struct {
	Source string
	Skill  string
	Path   string
	Reason string
}

// scanSource walks one source root and returns every skill it should
// contribute, plus the ones it dropped.
func scanSource(repoRoot string, src SourceConfig, m *Manifest) ([]Skill, []drop, error) {
	root := filepath.Join(repoRoot, src.Path, src.Root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, nil, fmt.Errorf("source %q: %w (did you run "+
			"`git submodule update --init --depth 1 --recursive`?)", src.Name, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("source %q: %s is not a directory", src.Name, root)
	}

	var found []Skill
	var drops []drop
	// Skill directories already accepted, so nested SKILL.md files can be
	// recognised and skipped — the outer skill's copy already contains them.
	var accepted []string

	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if p != root && m.Exclude.prunedDir(d.Name()) {
			return fs.SkipDir
		}
		if _, statErr := os.Stat(filepath.Join(p, skillFile)); statErr != nil {
			return nil
		}

		rel := relSlash(root, p)
		if outer, nested := nestedIn(accepted, rel); nested {
			drops = append(drops, drop{Source: src.Name, Path: rel,
				Reason: "nested inside skill " + outer})
			return nil
		}

		sk, parseErr := readSkill(p, rel, src)
		if parseErr != nil {
			drops = append(drops, drop{Source: src.Name, Path: rel,
				Reason: "unreadable: " + parseErr.Error()})
			return nil
		}
		if pat, ok := denied(m.Deny, sk.Name, rel); ok {
			drops = append(drops, drop{Source: src.Name, Skill: sk.Name, Path: rel,
				Reason: "deny rule " + pat})
			return nil
		}
		if pat, ok := denied(src.Deny, sk.Name, rel); ok {
			drops = append(drops, drop{Source: src.Name, Skill: sk.Name, Path: rel,
				Reason: "source deny rule " + pat})
			return nil
		}
		accepted = append(accepted, rel)
		found = append(found, sk)
		return nil
	})
	if walkErr != nil {
		return nil, nil, fmt.Errorf("source %q: %w", src.Name, walkErr)
	}

	sort.Slice(found, func(i, j int) bool { return found[i].RelPath < found[j].RelPath })
	return found, drops, nil
}

// readSkill parses one skill directory's SKILL.md into a Skill.
func readSkill(dir, rel string, src SourceConfig) (Skill, error) {
	raw, err := os.ReadFile(filepath.Join(dir, skillFile))
	if err != nil {
		return Skill{}, err
	}
	fm, body, lead := splitFrontMatter(raw)

	sk := Skill{
		Source:      src.Name,
		RelPath:     rel,
		AbsDir:      dir,
		Name:        strings.TrimSpace(fm.str("name")),
		Description: collapse(fm.str("description")),
		Tags:        fm.tags(),
	}
	if sk.Name == "" {
		sk.Name = filepath.Base(dir)
	}
	if sk.Description == "" {
		sk.Description = deriveDescription(body)
	}
	sk.Domain = resolveDomain(src, rel, fm)
	sk.Doc = normalizeDoc(fm, lead, body, sk)
	return sk, nil
}

// resolveDomain derives the grouping key used by the compact skill overview.
// The path-segment strategy needs at least one directory level above the skill
// itself; everything else falls back to front matter and finally to the source
// name, so every skill always lands in exactly one domain.
func resolveDomain(src SourceConfig, rel string, fm frontMatter) string {
	if src.DomainFrom == domainFromPathSegment {
		if seg, _, ok := strings.Cut(rel, "/"); ok && seg != "" {
			return slug(seg)
		}
	}
	for _, key := range []string{"domain", "category", "field"} {
		if v := fm.nested("metadata", key); v != "" {
			return slug(v)
		}
		if v := fm.str(key); v != "" {
			return slug(v)
		}
	}
	return slug(src.Name)
}

// leadingComment matches an HTML comment block at the very start of a file.
// Several upstream skills put a copyright banner above their YAML front matter,
// which makes strict parsers (including the framework's) see no front matter at
// all and fall back to a bare directory name with no description.
var leadingComment = regexp.MustCompile(`(?s)\A\s*<!--.*?-->\s*`)

// frontMatter is a parsed YAML front-matter block. Values stay as any so nested
// maps (clawbio's metadata.domain) and sequences (tags) survive.
type frontMatter map[string]any

func (f frontMatter) str(key string) string {
	if f == nil {
		return ""
	}
	s, _ := f[key].(string)
	return s
}

// submap reads f[key] as a nested mapping.
//
// Both branches are load-bearing: when decoding into a named map type, yaml.v3
// gives nested mappings that same named type (frontMatter) rather than a plain
// map[string]any, and a type assertion needs an identical type — so asserting
// only to map[string]any silently misses every nested value.
func (f frontMatter) submap(key string) frontMatter {
	if f == nil {
		return nil
	}
	switch v := f[key].(type) {
	case frontMatter:
		return v
	case map[string]any:
		return frontMatter(v)
	default:
		return nil
	}
}

// nested reads f[outer][inner] as a string.
func (f frontMatter) nested(outer, inner string) string {
	return f.submap(outer).str(inner)
}

// tags collects front-matter tags from either the top level or metadata.tags.
func (f frontMatter) tags() []string {
	if f == nil {
		return nil
	}
	raw := f["tags"]
	if raw == nil {
		if m := f.submap("metadata"); m != nil {
			raw = m["tags"]
		}
	}
	var out []string
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for s := range strings.SplitSeq(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// splitFrontMatter separates a leading HTML comment, the YAML front matter, and
// the Markdown body. Any of the three may be empty.
func splitFrontMatter(raw []byte) (fm frontMatter, body string, lead string) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if loc := leadingComment.FindStringIndex(text); loc != nil {
		lead = strings.TrimSpace(text[loc[0]:loc[1]])
		text = text[loc[1]:]
	}
	if !strings.HasPrefix(text, "---\n") {
		return nil, text, lead
	}
	// The terminator is a line that is exactly "---". Matching a bare "\n---"
	// prefix would also fire on a body's "-----" rule or a setext underline.
	end := strings.Index(text[3:], "\n---\n")
	closeLen := 5
	if end < 0 {
		// Front matter can also close at EOF with no trailing newline.
		if strings.HasSuffix(text, "\n---") {
			end = len(text) - 3 - 4
			closeLen = 4
		} else {
			return nil, text, lead
		}
	}
	block := text[4 : 3+end+1]
	rest := text[3+end+closeLen:]

	parsed := frontMatter{}
	if err := yaml.Unmarshal([]byte(block), &parsed); err != nil {
		// Some upstream front matter is not valid YAML — most often a plain
		// scalar description containing ": " ("sample-identity QC: getSex"),
		// which YAML reads as a nested mapping and rejects. Falling back to a
		// line-oriented parse recovers the real name and description; treating
		// the block as body text instead would lose the skill's identity and,
		// worse, re-emit a "name:" line inside its own description.
		return parseFrontMatterLines(block), rest, lead
	}
	return parsed, rest, lead
}

// parseFrontMatterLines parses a front-matter block one line at a time, for
// blocks that are not valid YAML. It understands "key: value" and block scalars
// ("key: |"), which covers everything the upstream libraries actually use.
func parseFrontMatterLines(block string) frontMatter {
	out := frontMatter{}
	lines := strings.Split(block, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		// Only top-level keys: an indented line belongs to the value above,
		// and treating it as a key is how a nested "name" ends up shadowing
		// the real one.
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		rawKey, rawVal, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(rawKey)
		val := strings.TrimSpace(rawVal)
		if !plainKey(key) {
			continue
		}
		if isBlockScalar(val) {
			var parts []string
			for i+1 < len(lines) {
				next := lines[i+1]
				if next != "" && next[0] != ' ' && next[0] != '\t' {
					break
				}
				parts = append(parts, strings.TrimSpace(next))
				i++
			}
			val = strings.TrimSpace(strings.Join(parts, " "))
		}
		out[key] = val
	}
	return out
}

// isBlockScalar reports whether a value introduces a YAML block scalar, whose
// content lives on the following indented lines.
func isBlockScalar(val string) bool {
	if val == "" {
		return false
	}
	return val[0] == '|' || val[0] == '>'
}

// normalizeDoc rewrites SKILL.md so its front matter is the first thing in the
// file, always carries a name and description, and is exactly one line per key.
//
// The one-line-per-key rule is not cosmetic. The framework's front-matter reader
// is line-oriented and FLATTENS nesting: it takes the text before the first colon
// on every line as a key, at any indentation. So re-emitting an upstream skill's
// nested block-style metadata has two failure modes, both silent —
//
//   - a nested "name:" or "description:" anywhere in the block overwrites the
//     real one, and skills that collide on the resulting name get dropped from
//     the repository index entirely;
//   - a value folded across lines (which yaml.Marshal does past ~80 columns) is
//     truncated at its first line.
//
// Emitting every value as a single-line JSON scalar — JSON being a subset of
// YAML — keeps the upstream metadata intact for readers that parse the block
// properly, while giving the line-oriented reader exactly what it expects.
//
// Any leading copyright banner is preserved verbatim, just moved below the front
// matter, since it is the banner sitting above the YAML that hides the front
// matter from strict readers in the first place.
func normalizeDoc(fm frontMatter, lead, body string, sk Skill) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	// name and description are emitted UNQUOTED. The framework's reader stores
	// a single-line value as raw text without YAML-unquoting it, so a quoted
	// name would be indexed with its quotes still attached and never match the
	// name the catalog advertises. plainScalar keeps them valid unquoted YAML.
	b.WriteString("name: " + plainScalar(sk.Name) + "\n")
	b.WriteString("description: " + plainScalar(sk.Description) + "\n")
	// Re-emit the upstream keys we did not synthesise, so nothing an
	// upstream skill declares (tool_type, primary_tool, license, …) is lost.
	keys := make([]string, 0, len(fm))
	for k := range fm {
		if k == "name" || k == "description" || !plainKey(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k + ": " + jsonScalar(fm[k]) + "\n")
	}
	b.WriteString("---\n")
	if lead != "" {
		b.WriteString("\n" + lead + "\n")
	}
	if body != "" {
		if !strings.HasPrefix(body, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(body)
	}
	return []byte(b.String())
}

// plainScalar renders a string as a valid single-line UNQUOTED YAML scalar.
//
// Quoting is not an option for name and description (see normalizeDoc), so the
// few constructs that would end a plain scalar early are neutralised instead:
//
//   - ": " would start a nested mapping — and if the text after it happens to be
//     "name" or "description", the framework's flattening reader would take it
//     as the skill's real key. The space is dropped ("QC: getSex" → "QC:getSex").
//   - " #" would start a comment, truncating the value.
//   - A leading YAML indicator character would change the value's type.
//
// The pristine text is kept in the catalog (index.json), which is what search
// and the prompt overview actually read, so this only touches the copy inside
// SKILL.md's front matter.
func plainScalar(s string) string {
	// Collapse first so a colon or hash split across lines is still seen, and
	// again afterwards to close the gaps the substitutions leave behind.
	s = collapse(s)
	s = strings.ReplaceAll(s, ": ", ":")
	s = strings.ReplaceAll(s, " #", " ")
	s = strings.TrimRight(s, ":")
	s = strings.TrimLeft(s, "-?:,[]{}#&*!|>'\"%@`")
	return collapse(s)
}

// jsonScalar renders a value as a single-line YAML-compatible scalar. JSON is
// valid YAML flow syntax, and json.Marshal never wraps or emits a bare newline,
// which is exactly the property the framework's line-oriented reader needs.
func jsonScalar(v any) string {
	// frontMatter is a named map type; convert so encoding/json sees a plain
	// map rather than reflecting over the alias.
	if m, ok := v.(frontMatter); ok {
		v = map[string]any(m)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%q", fmt.Sprint(v))
	}
	return string(out)
}

// plainKey reports whether a front-matter key is safe to re-emit as "key: value".
// A key containing a colon, a comment marker or leading/trailing space would
// change how the line parses, and no upstream skill needs one.
func plainKey(k string) bool {
	if strings.TrimSpace(k) != k || k == "" {
		return false
	}
	return !strings.ContainsAny(k, ":#\n\"'")
}

// deriveDescription synthesises a one-line summary from the Markdown body for
// skills whose front matter has none, so they are still findable by search.
func deriveDescription(body string) string {
	inFence := false
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		// Track fences so the first line of a code block is not mistaken
		// for prose.
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "---") || strings.HasPrefix(line, "<!--") ||
			strings.HasPrefix(line, ">") || strings.HasPrefix(line, "|") {
			continue
		}
		line = collapse(line)
		if len(line) > maxDerivedDescription {
			line = strings.TrimSpace(line[:maxDerivedDescription]) + "…"
		}
		return line
	}
	return ""
}

// collapse flattens whitespace so a description occupies a single line.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// slug normalises a domain label to lowercase-kebab.
func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "-", "_", "-", "/", "-").Replace(s)
	return strings.Trim(s, "-")
}

// relSlash returns p relative to base with forward slashes.
func relSlash(base, p string) string {
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// nestedIn reports whether rel sits inside one of the given directories.
func nestedIn(dirs []string, rel string) (string, bool) {
	for _, d := range dirs {
		if strings.HasPrefix(rel, d+"/") {
			return d, true
		}
	}
	return "", false
}
