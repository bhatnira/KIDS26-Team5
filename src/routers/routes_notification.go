package routers

import (
	v1 "antelope/routers/api/v1"

	"github.com/gin-gonic/gin"
)

func (rm *RouterManager) registerNotificationRoutes(priv *gin.RouterGroup, h *v1.NotificationHandler) {
	notif := priv.Group("/notifications")
	{
		notif.GET("", h.List)
		notif.PUT("/:id/read", h.MarkRead)
		notif.PUT("/read-all", h.MarkAllRead)
		notif.GET("/stream", h.Stream)
	}
}
