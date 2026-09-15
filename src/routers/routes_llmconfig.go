package routers

import (
	v1 "antelope/routers/api/v1"

	"github.com/gin-gonic/gin"
)

func (rm *RouterManager) registerLLMConfigRoutes(priv *gin.RouterGroup, h *v1.LLMConfigHandler) {
	llm := priv.Group("/llm")
	{
		llm.GET("/configs", h.ListConfigs)
		llm.POST("/configs", h.AddConfig)
		llm.PUT("/configs/:id", h.UpdateConfig)
		llm.DELETE("/configs/:id", h.DeleteConfig)
		llm.POST("/test-connection", h.TestConnection)
	}
}
