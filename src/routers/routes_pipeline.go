package routers

import (
	v1 "antelope/routers/api/v1"

	"github.com/gin-gonic/gin"
)

func (rm *RouterManager) registerPipelineRoutes(pub, priv *gin.RouterGroup, h *v1.PipelineHandler, adminOrSuperMW gin.HandlerFunc) {
	pub.GET("/pipeline/list", h.GetList)
	pub.GET("/pipeline/list_all", h.GetAll)
	priv.GET("/pipeline/schema", h.GetSchema)
	priv.POST("/pipeline/add", adminOrSuperMW, h.Add)
	priv.DELETE("/pipeline/delete", adminOrSuperMW, h.Delete)
	priv.POST("/pipeline/update", h.Update)
}

func (rm *RouterManager) registerTemplateRoutes(pub, priv *gin.RouterGroup, h *v1.JobTemplateHandler, adminOrSuperMW gin.HandlerFunc) {
	pub.GET("/templates", h.List)
	pub.GET("/templates/:id", h.Get)
	priv.POST("/templates", adminOrSuperMW, h.Create)
	priv.PUT("/templates/:id", adminOrSuperMW, h.Update)
	priv.DELETE("/templates/:id", adminOrSuperMW, h.Delete)
}
