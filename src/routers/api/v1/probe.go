package v1

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	nomad "github.com/hashicorp/nomad/api"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ProbeHandler handles health and readiness probe routes.
type ProbeHandler struct {
	db    *gorm.DB
	redis redis.UniversalClient
	nomad *nomad.Client
}

func NewProbeHandler(db *gorm.DB, redis redis.UniversalClient, nomad *nomad.Client) *ProbeHandler {
	return &ProbeHandler{db: db, redis: redis, nomad: nomad}
}

// @Summary Readiness probe
// @Description Readiness probe
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /ready [get]
func (h *ProbeHandler) Ready(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sql database connection failed"})
		return
	}
	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sql database ping failed"})
		return
	}

	if h.redis == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "redis connection failed"})
		return
	}
	if _, err := h.redis.Ping(context.Background()).Result(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "redis ping failed"})
		return
	}

	if h.nomad == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "nomad connection failed"})
		return
	}
	if _, err := h.nomad.Status().Leader(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "nomad status failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
