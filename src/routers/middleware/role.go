package middleware

import (
	"net/http"
	"slices"

	"antelope/pkg/response"
	authsvc "antelope/services/auth"

	"github.com/gin-gonic/gin"
)

// Role constants
const (
	RoleSuper = "super"
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// NewRoleMiddleware returns a middleware that enforces role-based access control.
//
// ARCH-1: The previous implementation fetched the user row from Postgres on every
// request to read the role, adding a round-trip even though the role is already
// embedded in the validated JWT claims set by NewAuthMiddleware. This version reads
// the role from the gin context (populated by NewAuthMiddleware) with no DB query.
func NewRoleMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := authsvc.GetUserID(c)
		if userID == 0 {
			abortUnauthorized(c)
			return
		}

		role := authsvc.GetUserRole(c)
		if role == RoleSuper {
			c.Next()
			return
		}

		if slices.Contains(allowedRoles, role) {
			c.Next()
			return
		}

		abortForbidden(c)
	}
}

// NewSetRoleMiddleware returns a middleware that loads and sets the user's role
// in the gin context without enforcing access control.
// The role comes from JWT claims — no DB query is required.
func NewSetRoleMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := authsvc.GetUserID(c)
		if userID == 0 {
			c.Set("userRole", RoleUser)
			c.Next()
			return
		}
		role := authsvc.GetUserRole(c)
		if role == "" {
			role = RoleUser
		}
		c.Set("userRole", role)
		c.Next()
	}
}

// GetUserRole returns the user role from gin context
func GetUserRole(c *gin.Context) string {
	if role, exists := c.Get("userRole"); exists {
		if roleStr, ok := role.(string); ok {
			return roleStr
		}
	}
	return RoleUser
}

// IsSuper checks if the current user is a super user
func IsSuper(c *gin.Context) bool {
	return GetUserRole(c) == RoleSuper
}

// IsAdmin checks if the current user is an admin
func IsAdmin(c *gin.Context) bool {
	return GetUserRole(c) == RoleAdmin
}

// IsAdminOrSuper checks if the current user is admin or super
func IsAdminOrSuper(c *gin.Context) bool {
	role := GetUserRole(c)
	return role == RoleSuper || role == RoleAdmin
}

func abortUnauthorized(c *gin.Context) {
	response.HandleResponse(c, response.ResponseStruct{
		HttpStatus: http.StatusUnauthorized,
		Code:       response.UnauthorizedCode,
		Data:       nil,
		Msg:        response.Unauthorized,
	})
	c.Abort()
}

func abortForbidden(c *gin.Context) {
	response.HandleResponse(c, response.ResponseStruct{
		HttpStatus: http.StatusForbidden,
		Code:       response.ForbiddenCode,
		Data:       nil,
		Msg:        response.Forbidden,
	})
	c.Abort()
}
