// Package apikey implements personal API-key management. An API key is a
// long-lived JWT access token; this service mints it via the auth service,
// persists only metadata, and revokes keys through the shared session store.
package apikey

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"antelope/internal/modules/auth/session"
	"antelope/internal/modules/log"
	"antelope/models"
	"antelope/pkg/apperr"
	"antelope/pkg/response"
	"antelope/pkg/types"
	authsvc "antelope/services/auth"
)

// prefixLen is how many leading characters of the raw token are stored as a
// non-sensitive display hint so users can tell keys apart in the list.
const prefixLen = 12

type Service interface {
	Generate(userID uint, dto types.APIKeyGenerateDto) (gin.H, error)
	List(userID uint) (gin.H, error)
	Revoke(ctx context.Context, userID, id uint) error
}

type service struct {
	db           *gorm.DB
	authSvc      authsvc.Service
	sessionStore session.Store
}

func NewService(db *gorm.DB, authSvc authsvc.Service, sessionStore session.Store) Service {
	return &service{db: db, authSvc: authSvc, sessionStore: sessionStore}
}

// APIKeyDTO is the safe, listable view of an API key (never exposes the token or JTI).
type APIKeyDTO struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Expired   bool      `json:"expired"`
}

func (s *service) Generate(userID uint, dto types.APIKeyGenerateDto) (gin.H, error) {
	// Load fresh identity so the key carries the user's current email and role.
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, apperr.Unauthorized(response.UserNotExist)
	}

	ttl := time.Duration(dto.ExpireDays) * 24 * time.Hour
	token, jti, expiresAt, err := s.authSvc.IssueAPIKeyToken(authsvc.BaseClaims{
		ID: user.ID, Email: user.Email, Role: user.Role,
	}, ttl)
	if err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}

	prefix := token
	if len(prefix) > prefixLen {
		prefix = prefix[:prefixLen] + "…"
	}

	key := models.APIKey{
		UserId:    userID,
		Name:      dto.Name,
		JTI:       jti,
		Prefix:    prefix,
		ExpiresAt: expiresAt,
	}
	if err := s.db.Create(&key).Error; err != nil {
		log.L().Error("create api key failed", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}

	// The raw token is returned here once and never persisted or shown again.
	return gin.H{
		"id":         key.ID,
		"name":       key.Name,
		"key":        token,
		"expires_at": expiresAt,
	}, nil
}

func (s *service) List(userID uint) (gin.H, error) {
	var keys []models.APIKey
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}

	now := time.Now()
	dtos := make([]APIKeyDTO, len(keys))
	for i, k := range keys {
		dtos[i] = APIKeyDTO{
			ID:        k.ID,
			Name:      k.Name,
			Prefix:    k.Prefix,
			CreatedAt: k.CreatedAt,
			ExpiresAt: k.ExpiresAt,
			Expired:   now.After(k.ExpiresAt),
		}
	}
	return gin.H{"items": dtos}, nil
}

func (s *service) Revoke(ctx context.Context, userID, id uint) error {
	var key models.APIKey
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.NotFound("api key not found")
		}
		return apperr.ServerError(response.SystemError)
	}

	// Block the underlying JWT until it would have expired anyway. Skip if the
	// key is already past expiry (the token is invalid on its own).
	if ttl := time.Until(key.ExpiresAt); ttl > 0 {
		if err := s.sessionStore.Revoke(ctx, key.JTI, ttl); err != nil {
			log.L().Error("revoke api key jti failed", zap.String("jti", key.JTI), zap.Error(err))
			return apperr.ServerError(response.SystemError)
		}
	}

	if err := s.db.Delete(&key).Error; err != nil {
		return apperr.ServerError(response.SystemError)
	}
	return nil
}
