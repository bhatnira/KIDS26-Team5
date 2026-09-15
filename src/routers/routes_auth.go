package routers

import (
	v1 "antelope/routers/api/v1"

	"github.com/gin-gonic/gin"
)

func (rm *RouterManager) registerAuthRoutes(pub, priv *gin.RouterGroup, h *v1.AuthHandler) {
	pub.POST("/auth/login", h.LocalLogin)
	pub.POST("/auth/refresh", h.RefreshToken)
	pub.POST("/auth/ldap/login", h.LdapLogin)
	pub.GET("/auth/oidc/:provider/start", h.OidcStart)
	pub.POST("/auth/oidc/callback", h.OidcCallback)
	pub.GET("/auth/providers", h.GetEnabledProviders)
	priv.POST("/auth/logout", h.Logout)
}
