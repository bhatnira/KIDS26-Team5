package routers

import (
	"github.com/gin-gonic/gin"

	v1 "antelope/routers/api/v1"
)

func (rm *RouterManager) registerAPIKeyRoutes(priv *gin.RouterGroup, h *v1.APIKeyHandler) {
	keys := priv.Group("/user/api-keys")
	{
		keys.GET("", h.List)
		keys.POST("", h.Generate)
		keys.DELETE("/:id", h.Revoke)
	}
}
