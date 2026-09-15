package middleware

import (
	"net/http"

	"antelope/internal/modules/setting"

	"github.com/gin-gonic/gin"
)

const (
	CORSAllowAll        = "allow-all"
	CORSWhitelist       = "whitelist"
	CORSStrictWhitelist = "strict-whitelist"
)

func CORSMiddlewareAllowAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Headers", "Content-Type,AccessToken,X-CSRF-Token, Cache-Control, Authorization, Token,X-Token,X-User-ID")
		c.Header("Access-Control-Allow-Methods", "POST,GET,OPTIONS,PUT,DELETE")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type, New-Token, New-Expires-At")
		c.Header("Access-Control-Allow-Credentials", "true")

		if method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
		}
		c.Next()
	}
}

// CORSMiddleware returns a CORS middleware configured from the provided CORSConfig.
func CORSMiddleware(cfg ...setting.CORSConfig) gin.HandlerFunc {
	var corsCfg setting.CORSConfig
	if len(cfg) > 0 {
		corsCfg = cfg[0]
	}

	if corsCfg.Mode == "" || corsCfg.Mode == CORSAllowAll {
		return CORSMiddlewareAllowAll()
	}

	return func(c *gin.Context) {
		whitelist := checkCors(c.GetHeader("origin"), corsCfg)

		if whitelist != nil {
			c.Header("Access-Control-Allow-Origin", whitelist.AllowOrigin)
			c.Header("Access-Control-Allow-Headers", whitelist.AllowHeaders)
			c.Header("Access-Control-Allow-Methods", whitelist.AllowMethods)
			c.Header("Access-Control-Expose-Headers", whitelist.ExposeHeaders)
			if whitelist.AllowCredentials {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}

		if whitelist == nil && corsCfg.Mode == CORSStrictWhitelist && !(c.Request.Method == http.MethodGet && c.Request.URL.Path == "/health") {
			c.AbortWithStatus(http.StatusForbidden)
		} else if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
		}

		c.Next()
	}
}

func checkCors(origin string, cfg setting.CORSConfig) *setting.CORSWhitelistConfig {
	for _, whitelist := range cfg.Whitelist {
		if origin == whitelist.AllowOrigin {
			return &whitelist
		}
	}
	return nil
}
