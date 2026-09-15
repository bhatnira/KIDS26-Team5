package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"antelope/internal/modules/agent/skillbundle"
	"antelope/internal/modules/log"
	"antelope/models"
	"antelope/pkg/apperr"
	"antelope/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	// skillListDefaultLimit bounds an unfiltered listing. The built-in library
	// is ~700 skills, so the endpoint pages rather than returning everything.
	skillListDefaultLimit = 50
	// skillListMaxLimit caps what one request may ask for.
	skillListMaxLimit = 200
	// skillBodyMaxBytes bounds the SKILL.md body returned by GetSkill.
	skillBodyMaxBytes = 256 << 10
)

// SkillQuery filters a skill listing.
type SkillQuery struct {
	// Search is free text matched against name, description, domain and tags.
	Search string
	// Domain restricts results to one domain.
	Domain string
	// Scope is "", "global" or "user".
	Scope  string
	Limit  int
	Offset int
}

// ListSkills returns the union of the built-in skill library and the requesting
// user's own uploaded skills.
//
// Built-in skills come from the bundle catalog held in memory rather than from
// the database: the library ships with the binary, so the image tag is what
// version-controls it, and mirroring ~700 rows into Postgres on every boot would
// only create a second source of truth that can drift. User skills stay in the
// database, where they are actually owned and mutable.
//
// Filtering, sorting and paging all happen server-side because the catalog is
// far too large for the client to hold and filter.
func (s *service) ListSkills(ctx context.Context, userID uint, q SkillQuery) (gin.H, error) {
	if userID == 0 {
		return nil, errUserIDRequired
	}

	var items []gin.H
	catalog := s.agent.SkillLibrary().Catalog()

	if q.Scope != "user" {
		items = append(items, catalogItems(catalog, q)...)
	}
	if q.Scope != "global" {
		rows, err := s.userSkillRows(ctx, userID)
		if err != nil {
			return nil, err
		}
		items = append(items, userSkillItems(rows, q)...)
	}

	// User skills first: there are few of them and they are what the user just
	// added, so they should not be buried under the built-in library.
	sort.SliceStable(items, func(i, j int) bool {
		gi, _ := items[i]["is_global"].(bool)
		gj, _ := items[j]["is_global"].(bool)
		if gi != gj {
			return !gi
		}
		return false
	})

	total := len(items)
	items = page(items, q)

	return gin.H{
		"items":   items,
		"total":   total,
		"limit":   effectiveLimit(q.Limit),
		"offset":  max(q.Offset, 0),
		"domains": domainFacets(catalog),
		"library": libraryInfo(s.agent.SkillLibrary()),
	}, nil
}

// GetSkill returns metadata plus the SKILL.md body for a single skill, looking in
// the built-in library first and then the user's own skills.
func (s *service) GetSkill(ctx context.Context, userID uint, name string) (gin.H, error) {
	if userID == 0 {
		return nil, errUserIDRequired
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.BadRequest(response.FailCode, "skill name is required")
	}

	lib := s.agent.SkillLibrary()
	if entry, ok := lib.Catalog().Lookup(name); ok {
		item := gin.H{
			"name":        entry.Name,
			"domain":      entry.Domain,
			"description": entry.Description,
			"source":      entry.Source,
			"tags":        entry.Tags,
			"is_global":   true,
		}
		if body, err := readSkillDoc(lib.Root(), entry.Dir); err != nil {
			// The catalog is authoritative for metadata, so a missing file is
			// worth logging but not worth failing the request over — the
			// caller still gets everything the list view shows.
			log.L().Warn("read built-in SKILL.md failed",
				zap.String("skill", entry.Name), zap.Error(err))
		} else {
			item["body"] = body
		}
		return item, nil
	}

	var row models.AgentSkill
	err := s.db.WithContext(ctx).
		Where("name = ? AND user_id = ?", name, userID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NotFound("skill not found")
		}
		log.L().Error("load skill failed", zap.Error(err))
		return nil, apperr.ServerError("failed to load skill")
	}
	return userSkillItem(row), nil
}

// userSkillRows loads the user's own skill rows. Rows with a NULL user_id are
// legacy platform-wide entries from before the library shipped in the binary;
// they are included so an existing deployment's data stays visible.
func (s *service) userSkillRows(ctx context.Context, userID uint) ([]models.AgentSkill, error) {
	var rows []models.AgentSkill
	err := s.db.WithContext(ctx).
		Where("user_id = ? OR user_id IS NULL", userID).
		Order("name ASC").
		Find(&rows).Error
	if err != nil {
		log.L().Error("list skills failed", zap.Error(err))
		return nil, apperr.ServerError("failed to list skills")
	}
	return rows, nil
}

// catalogItems applies the query to the built-in catalog.
func catalogItems(catalog *skillbundle.Catalog, q SkillQuery) []gin.H {
	if catalog == nil || catalog.Len() == 0 {
		return nil
	}
	var entries []skillbundle.Entry
	switch {
	case q.Search != "":
		// Ask for everything and page afterwards, so paging past the first
		// screen of results works.
		for _, hit := range catalog.Search(q.Search, catalog.Len()) {
			entries = append(entries, hit.Entry)
		}
	case q.Domain != "":
		entries = catalog.InDomain(q.Domain)
	default:
		entries = catalog.Entries()
	}

	out := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		// Search ranks across all domains, so the domain filter is applied
		// afterwards when both are set.
		if q.Domain != "" && !strings.EqualFold(e.Domain, q.Domain) {
			continue
		}
		out = append(out, gin.H{
			"name":        e.Name,
			"domain":      e.Domain,
			"description": e.Description,
			"source":      e.Source,
			"tags":        e.Tags,
			"is_global":   true,
		})
	}
	return out
}

// userSkillItems applies the query to the user's own skills. The catalog has a
// real search index; this set is small enough for a substring match.
func userSkillItems(rows []models.AgentSkill, q SkillQuery) []gin.H {
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		if q.Domain != "" && !strings.EqualFold(r.Domain, q.Domain) {
			continue
		}
		if q.Search != "" && !matchesAny(q.Search, r.Name, r.Description, r.Domain) {
			continue
		}
		out = append(out, userSkillItem(r))
	}
	return out
}

func userSkillItem(r models.AgentSkill) gin.H {
	return gin.H{
		"id":          r.ID,
		"name":        r.Name,
		"domain":      r.Domain,
		"version":     r.Version,
		"description": r.Description,
		"size_bytes":  r.SizeBytes,
		"is_global":   r.UserID == nil,
	}
}

// domainFacets lists the built-in domains and their counts so the UI can offer a
// filter without downloading the whole catalog.
func domainFacets(catalog *skillbundle.Catalog) []gin.H {
	domains := catalog.Domains()
	out := make([]gin.H, 0, len(domains))
	for _, d := range domains {
		out = append(out, gin.H{"domain": d.Domain, "count": d.Skills})
	}
	return out
}

// libraryInfo describes the loaded built-in library, including per-source
// licensing, so the settings page can attribute what it is showing.
func libraryInfo(lib *skillbundle.Library) gin.H {
	sources := lib.Catalog().Sources()
	out := make([]gin.H, 0, len(sources))
	for _, src := range sources {
		out = append(out, gin.H{
			"name":    src.Name,
			"url":     src.URL,
			"license": src.License,
			"commit":  src.Commit,
			"skills":  src.Skills,
		})
	}
	return gin.H{
		"skills":   lib.Len(),
		"embedded": lib.Embedded(),
		"sources":  out,
	}
}

// readSkillDoc reads a bundled SKILL.md, bounded so an unusually large one
// cannot be used to pull megabytes through the API.
//
// dir comes from the generated catalog rather than from user input, but it is
// still confined to the bundle root: a hand-edited index.json must not be able
// to read arbitrary files off the host.
func readSkillDoc(root, dir string) (string, error) {
	if root == "" || dir == "" {
		return "", errors.New("skill bundle root is unknown")
	}
	full := filepath.Join(root, filepath.FromSlash(dir), "SKILL.md")
	rel, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("skill path escapes the bundle root")
	}
	f, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, skillBodyMaxBytes)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return "", err
	}
	return string(buf[:n]), nil
}

func matchesAny(needle string, fields ...string) bool {
	needle = strings.ToLower(needle)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), needle) {
			return true
		}
	}
	return false
}

func effectiveLimit(limit int) int {
	if limit <= 0 {
		return skillListDefaultLimit
	}
	return min(limit, skillListMaxLimit)
}

// page slices the result set, tolerating an offset past the end.
func page(items []gin.H, q SkillQuery) []gin.H {
	offset := max(q.Offset, 0)
	if offset >= len(items) {
		return []gin.H{}
	}
	items = items[offset:]
	if limit := effectiveLimit(q.Limit); len(items) > limit {
		items = items[:limit]
	}
	return items
}
