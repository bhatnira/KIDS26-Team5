package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"antelope/internal/modules/log"
)

// RequestLogger attaches a request-scoped logger to the context and logs
// each completed request. Downstream handlers and services can retrieve the
// enriched logger via log.FromCtx(ctx).
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Header("X-Request-ID", requestID)

		logger := log.L().With(
			zap.String("request_id", requestID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("client_ip", c.ClientIP()),
		)

		ctx := log.WithContext(c.Request.Context(), logger)
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		logger.Info("request completed",
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.Int("size", c.Writer.Size()),
		)
	}
}
