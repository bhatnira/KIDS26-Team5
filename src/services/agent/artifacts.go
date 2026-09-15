package agent

import (
	"context"
	"strconv"
	"time"

	"antelope/pkg/apperr"

	"github.com/gin-gonic/gin"
	"trpc.group/trpc-go/trpc-agent-go/artifact"
)

// defaultPresignedTTL is the fallback download URL TTL when the platform
// config doesn't override it.
const defaultPresignedTTL = time.Hour

// ListArtifacts returns the names of every artifact saved during the
// session (one entry per logical filename, not per version).
func (s *service) ListArtifacts(ctx context.Context, userID uint, sessionID string) (gin.H, error) {
	if userID == 0 {
		return nil, errUserIDRequired
	}
	if sessionID == "" {
		return nil, apperr.BadRequest(400, "session id is required")
	}
	info := artifact.SessionInfo{
		AppName:   s.cfg.AppName,
		UserID:    userIDToString(userID),
		SessionID: sessionID,
	}
	names, err := s.agent.Artifacts().ListArtifactKeys(ctx, info)
	if err != nil {
		return nil, apperr.ServerError(err.Error())
	}
	items := make([]gin.H, 0, len(names))
	for _, n := range names {
		versions, err := s.agent.Artifacts().ListVersions(ctx, info, n)
		if err != nil {
			continue
		}
		latest := -1
		if len(versions) > 0 {
			latest = versions[len(versions)-1]
		}
		items = append(items, gin.H{
			"name":           n,
			"latest_version": latest,
			"versions":       versions,
		})
	}
	return gin.H{"items": items}, nil
}

// GetArtifactDownloadURL returns a short-lived presigned URL the
// frontend can hit directly.
func (s *service) GetArtifactDownloadURL(ctx context.Context, userID uint, sessionID, name string, version int) (gin.H, error) {
	if userID == 0 {
		return nil, errUserIDRequired
	}
	if sessionID == "" || name == "" {
		return nil, apperr.BadRequest(400, "session id and name are required")
	}
	info := artifact.SessionInfo{
		AppName:   s.cfg.AppName,
		UserID:    userIDToString(userID),
		SessionID: sessionID,
	}

	// version<0 means "latest"; resolve via ListVersions.
	if version < 0 {
		versions, err := s.agent.Artifacts().ListVersions(ctx, info, name)
		if err != nil || len(versions) == 0 {
			return nil, apperr.NotFound("artifact not found")
		}
		version = versions[len(versions)-1]
	}

	ttl := defaultPresignedTTL
	if v := s.cfg.Artifacts.PresignedTTLSeconds; v > 0 {
		ttl = time.Duration(v) * time.Second
	}
	url, err := s.agent.Artifacts().PresignedURL(ctx, info, name, version, ttl)
	if err != nil {
		return nil, apperr.ServerError(err.Error())
	}
	return gin.H{
		"url":     url,
		"version": version,
		"expires": time.Now().Add(ttl).UTC(),
		"key":     name + "/" + strconv.Itoa(version),
	}, nil
}
