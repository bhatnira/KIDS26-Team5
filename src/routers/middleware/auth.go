package middleware

import (
	"errors"
	"net/http"

	"antelope/internal/modules/auth/jwt"
	"antelope/internal/modules/auth/session"
	"antelope/internal/modules/setting"
	"antelope/pkg/response"
	authsvc "antelope/services/auth"

	"github.com/gin-gonic/gin"
)

// NewAuthMiddleware returns a gin middleware that validates the Bearer access token
// and rejects revoked JTIs. The session store is injected explicitly so no global
// init is required.
func NewAuthMiddleware(jwtCfg setting.JwtConfig, store session.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := authsvc.GetToken(c)
		if token == "" {
			response.HandleResponse(c, response.ResponseStruct{
				HttpStatus: http.StatusUnauthorized,
				Code:       response.UnauthorizedCode,
				Data:       nil,
				Msg:        response.Unauthorized,
			})
			c.Abort()
			return
		}

		j := jwt.NewJWT(jwtCfg.AccessSigningKey, jwtCfg.RefreshSigningKey)
		claims, err := jwt.ParseAccess[authsvc.CustomClaims, *authsvc.CustomClaims](j, token)
		if err != nil {
			code := response.UnauthorizedCode
			msg := err.Error()
			if errors.Is(err, jwt.ErrTokenExpired) {
				code = response.TokenExpiredCode
			}
			response.HandleResponse(c, response.ResponseStruct{
				HttpStatus: http.StatusUnauthorized,
				Code:       code,
				Data:       nil,
				Msg:        msg,
			})
			c.Abort()
			return
		}

		// Reject tokens whose JTI has been revoked (e.g. via logout).
		if claims.JTI != "" && store.IsRevoked(c.Request.Context(), claims.JTI) {
			response.HandleResponse(c, response.ResponseStruct{
				HttpStatus: http.StatusUnauthorized,
				Code:       response.UnauthorizedCode,
				Data:       nil,
				Msg:        response.TokenInvalid,
			})
			c.Abort()
			return
		}

		// Reject tokens force-revoked by a role/status change or password reset:
		// the user's epoch has advanced past the one stamped into this token.
		// Fails open on a Redis error (the JWT signature + expiry still apply),
		// matching the IsRevoked posture.
		if current, epErr := store.CurrentEpoch(c.Request.Context(), claims.BaseClaims.ID); epErr == nil && claims.Epoch < current {
			response.HandleResponse(c, response.ResponseStruct{
				HttpStatus: http.StatusUnauthorized,
				Code:       response.UnauthorizedCode,
				Data:       nil,
				Msg:        response.TokenInvalid,
			})
			c.Abort()
			return
		}

		c.Set("claims", claims)
		c.Next()
	}
}
