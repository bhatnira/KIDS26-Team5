package agent

import (
	"context"
	"errors"
	"strings"

	"antelope/internal/modules/log"
	"antelope/models"
	"antelope/pkg/apperr"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// clearSentinel is what the client sends to wipe an existing credential
// without having to retype it.
const clearSentinel = "<clear>"

// WorkspaceConfigDto carries every field the frontend can send for an
// agent workspace settings update.
type WorkspaceConfigDto struct {
	Bucket        string `json:"bucket"`
	DaytonaAPIKey string `json:"daytona_api_key"`
	DaytonaAPIURL string `json:"daytona_api_url"`
}

// GetWorkspaceConfig returns the user's workspace bucket plus whether
// Daytona credentials are on file. Credential values are never returned.
func (s *service) GetWorkspaceConfig(ctx context.Context, userID uint) (gin.H, error) {
	if userID == 0 {
		return nil, errUserIDRequired
	}
	var ws models.AgentWorkspaceConfig
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&ws).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gin.H{"configured": false}, nil
		}
		log.L().Error("load workspace config failed", zap.Error(err))
		return nil, apperr.ServerError("failed to load workspace config")
	}

	return gin.H{
		"configured":          ws.Bucket != "" && ws.DaytonaAPIKey != "",
		"bucket":              ws.Bucket,
		"daytona_api_key_set": ws.DaytonaAPIKey != "",
		"daytona_api_url":     ws.DaytonaAPIURL,
		"updated_at":          ws.UpdatedAt,
	}, nil
}

// UpdateWorkspaceConfig upserts a row from the user's form. Empty
// credential strings mean "leave existing untouched"; the clearSentinel
// means "wipe".
func (s *service) UpdateWorkspaceConfig(ctx context.Context, userID uint, dto WorkspaceConfigDto) (gin.H, error) {
	if userID == 0 {
		return nil, errUserIDRequired
	}
	bucket := strings.TrimSpace(dto.Bucket)
	if bucket == "" {
		return nil, apperr.BadRequest(400, "bucket is required")
	}

	var ws models.AgentWorkspaceConfig
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&ws).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		storedKey, keyEncrypted, encErr := s.resolveDaytonaKey("", false, dto.DaytonaAPIKey)
		if encErr != nil {
			log.L().Error("encrypt daytona key failed", zap.Error(encErr))
			return nil, apperr.ServerError("failed to save workspace config")
		}
		ws = models.AgentWorkspaceConfig{
			UserID:              userID,
			Bucket:              bucket,
			DaytonaAPIKey:       storedKey,
			DaytonaKeyEncrypted: keyEncrypted,
			DaytonaAPIURL:       strings.TrimSpace(dto.DaytonaAPIURL),
		}
		if err := s.db.WithContext(ctx).Create(&ws).Error; err != nil {
			log.L().Error("create workspace config failed", zap.Error(err))
			return nil, apperr.ServerError("failed to save workspace config")
		}
	case err != nil:
		log.L().Error("load workspace config failed", zap.Error(err))
		return nil, apperr.ServerError("failed to load workspace config")
	default:
		storedKey, keyEncrypted, encErr := s.resolveDaytonaKey(ws.DaytonaAPIKey, ws.DaytonaKeyEncrypted, dto.DaytonaAPIKey)
		if encErr != nil {
			log.L().Error("encrypt daytona key failed", zap.Error(encErr))
			return nil, apperr.ServerError("failed to save workspace config")
		}
		ws.Bucket = bucket
		ws.DaytonaAPIKey = storedKey
		ws.DaytonaKeyEncrypted = keyEncrypted
		if u := strings.TrimSpace(dto.DaytonaAPIURL); u != "" || dto.DaytonaAPIURL == clearSentinel {
			if dto.DaytonaAPIURL == clearSentinel {
				ws.DaytonaAPIURL = ""
			} else {
				ws.DaytonaAPIURL = u
			}
		}
		if err := s.db.WithContext(ctx).Save(&ws).Error; err != nil {
			log.L().Error("update workspace config failed", zap.Error(err))
			return nil, apperr.ServerError("failed to save workspace config")
		}
	}

	// Bucket may have changed — invalidate the artifact service's cache
	// so the next chat turn picks up the new value immediately.
	s.agent.Artifacts().InvalidateBucket(userID)

	return gin.H{
		"configured": ws.Bucket != "" && ws.DaytonaAPIKey != "",
		"bucket":     ws.Bucket,
	}, nil
}

// resolveDaytonaKey resolves the three-state credential input the frontend
// sends and encrypts a newly supplied key at rest. (current, currentEncrypted)
// describe what is already stored; they are returned unchanged on "keep".
//
//	""              → keep current (stored form untouched)
//	"<clear>"       → wipe
//	anything else   → encrypt (when a key is configured) and mark encrypted
//
// When no encryption key is configured (SecretBox nil) the value is persisted
// as plaintext — still durable — matching the storage/LLM secret managers.
func (s *service) resolveDaytonaKey(current string, currentEncrypted bool, incoming string) (string, bool, error) {
	switch incoming {
	case "":
		return current, currentEncrypted, nil
	case clearSentinel:
		return "", false, nil
	default:
		box := s.agent.SecretBox()
		if box == nil {
			return incoming, false, nil
		}
		enc, err := box.Encrypt([]byte(incoming))
		if err != nil {
			return "", false, err
		}
		return enc, true, nil
	}
}
