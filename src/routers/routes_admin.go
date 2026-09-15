package routers

import (
	"net/http"

	v1 "antelope/routers/api/v1"

	"github.com/gin-gonic/gin"
)

func (rm *RouterManager) registerHealthRoutes(pub *gin.RouterGroup, probeH *v1.ProbeHandler) {
	pub.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, "ok") })
	pub.GET("/ready", probeH.Ready)
}

func (rm *RouterManager) registerAdminRoutes(priv *gin.RouterGroup, dashH *v1.DashboardHandler, adminOrSuperMW gin.HandlerFunc) {
	admin := priv.Group("/admin")
	admin.Use(adminOrSuperMW)
	{
		admin.GET("/dashboard/stats", dashH.GetAdminStats)
	}
}

func (rm *RouterManager) registerSystemRoutes(priv *gin.RouterGroup, userH *v1.UserHandler, authH *v1.AuthHandler, superOnlyMW gin.HandlerFunc) {
	sys := priv.Group("/system")
	sys.Use(superOnlyMW)
	{
		sys.GET("/user/list", userH.List)
		sys.POST("/user/add", userH.Add)
		sys.POST("/user/update", userH.Update)
		sys.DELETE("/user/delete/:id", userH.Delete)
		sys.GET("/auth/providers", authH.GetAllProviders)
		sys.POST("/auth/provider/add", authH.AddProvider)
		sys.POST("/auth/provider/update", authH.UpdateProvider)
		sys.DELETE("/auth/provider/delete/:id", authH.DeleteProvider)
		sys.POST("/auth/provider/test/:id", authH.TestProvider)
	}
}

func (rm *RouterManager) registerAgentRoutes(priv *gin.RouterGroup, h *v1.AgentHandler) {
	// Chat surface — same paths as the legacy /chat group so existing
	// frontend clients keep working. Session IDs are now strings.
	chat := priv.Group("/chat")
	{
		chat.POST("/conversations", h.CreateConversation)
		chat.GET("/conversations", h.ListConversations)
		chat.GET("/conversations/:id", h.GetConversation)
		chat.DELETE("/conversations/:id", h.DeleteConversation)
		chat.POST("/conversations/:id/messages", h.SendMessage)
	}

	// Agent settings + artifact surfaces.
	a := priv.Group("/agent")
	{
		a.GET("/workspace-config", h.GetWorkspaceConfig)
		a.PUT("/workspace-config", h.UpdateWorkspaceConfig)

		a.GET("/mcp-configs", h.ListMCPConfigs)
		a.POST("/mcp-configs", h.AddMCPConfig)
		a.PUT("/mcp-configs/:id", h.UpdateMCPConfig)
		a.DELETE("/mcp-configs/:id", h.DeleteMCPConfig)

		a.GET("/skills", h.ListSkills)
		a.GET("/skills/:name", h.GetSkill)

		a.GET("/sessions/:id/artifacts", h.ListArtifacts)
		a.GET("/sessions/:id/artifacts/*key", h.DownloadArtifact)
	}
}

func (rm *RouterManager) registerProxyRoutes(priv *gin.RouterGroup) {
	priv.GET("/cab/fastqQuery", v1.ProxyFastqQuery)
	priv.POST("/cab/pipeline", v1.ProxyPipelineSubmit)
}
