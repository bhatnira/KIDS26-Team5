package local

import (
	"errors"

	"antelope/internal/modules/misc"
	"antelope/models"
	"antelope/services/auth/source"

	"gorm.io/gorm"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserDisabled       = errors.New("account is disabled")
	ErrUserNotFound       = errors.New("user not found")
)

// LocalConfig is stateless — local auth requires no external configuration.
type LocalConfig struct{}

// LocalSubject carries the credentials submitted by the user.
type LocalSubject struct {
	Email    string
	Password string
}

// compile-time interface assertion
var _ source.Provider[LocalConfig, LocalSubject] = (*LocalProvider)(nil)

// LocalProvider implements source.Provider for password-based (local) authentication.
// It depends on an injected *gorm.DB so it remains testable without global state.
type LocalProvider struct {
	db *gorm.DB
}

func NewLocalProvider(db *gorm.DB) *LocalProvider {
	return &LocalProvider{db: db}
}

func (p *LocalProvider) Connect(_ LocalConfig) error   { return nil }
func (p *LocalProvider) Configured() bool              { return p.db != nil }
func (p *LocalProvider) SetConfig(_ LocalConfig) error { return nil }
func (p *LocalProvider) DeleteConfig()                 {}

// Verify checks the supplied credentials and returns the user's email on success.
func (p *LocalProvider) Verify(s LocalSubject) (string, error) {
	user, err := p.load(s)
	if err != nil {
		return "", err
	}
	return user.Email, nil
}

// VerifyAndLoad authenticates the subject and returns the full User record.
// Use this when the service needs the user object immediately after login.
func (p *LocalProvider) VerifyAndLoad(s LocalSubject) (*models.User, error) {
	return p.load(s)
}

func (p *LocalProvider) load(s LocalSubject) (*models.User, error) {
	var user models.User
	if err := p.db.Where("email = ?", s.Email).First(&user).Error; err != nil {
		return nil, ErrUserNotFound
	}
	if user.Status == 0 {
		return nil, ErrUserDisabled
	}
	if !misc.BcryptCheck(s.Password, user.Password) {
		return nil, ErrInvalidCredentials
	}
	return &user, nil
}
