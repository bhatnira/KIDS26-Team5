package agent

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"antelope/internal/modules/setting"

	"gorm.io/gorm"
)

// SkillSync resolves the per-user skill directories a request should load in
// addition to the built-in library.
//
// The built-in library is NOT its concern: that is loaded once at start-up by
// skillbundle (from the binary's embedded copy or from disk) and shared across
// all users, because re-scanning ~700 skill directories per request would be
// wasted work. This type only handles the small, per-user tree.
//
// V1 behavior: the per-user cache root (host dir into which user-uploaded skill
// bundles get lazy-synced from object storage) is returned if it exists. The
// actual S3-to-disk sync lands with the skills upload feature; until then this
// only surfaces what is already on disk.
type SkillSync struct {
	cfg setting.AgentSkillsConfig
	db  *gorm.DB
}

// NewSkillSync constructs a SkillSync. db is taken so the future S3 sync
// step can read AgentSkill rows; nil is tolerated to support tests that
// don't exercise per-user skills.
func NewSkillSync(cfg setting.AgentSkillsConfig, db *gorm.DB) *SkillSync {
	return &SkillSync{cfg: cfg, db: db}
}

// Resolve returns the user's own skill directories. Missing directories are
// silently dropped — the framework's skill.FSRepository tolerates that, and a
// user who has never uploaded a skill is the common case.
func (s *SkillSync) Resolve(_ context.Context, userID uint) ([]string, error) {
	base := strings.TrimSpace(s.cfg.UserCacheRoot)
	if base == "" || userID == 0 {
		return nil, nil
	}
	dir, err := userSkillDir(base, userID)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, nil
	}
	return []string{dir}, nil
}

// EnsureUserCacheDir creates the user-skill cache directory if it doesn't
// exist. Called by the (future) S3 sync path before writing bundles.
func (s *SkillSync) EnsureUserCacheDir(userID uint) (string, error) {
	base := strings.TrimSpace(s.cfg.UserCacheRoot)
	if base == "" {
		return "", nil
	}
	dir, err := userSkillDir(base, userID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func userSkillDir(base string, userID uint) (string, error) {
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	return filepath.Join(abs, strconv.FormatUint(uint64(userID), 10)), nil
}
