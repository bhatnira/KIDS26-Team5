package auth

import (
	"errors"
	"strings"

	"antelope/internal/modules/log"

	"github.com/gin-gonic/gin"
	gojwt "github.com/golang-jwt/jwt/v5"
)

const (
	authorizationHeader = "Authorization"
	authorizationScheme = "Bearer"
)

// BaseClaims holds the core user identity embedded in every token.
type BaseClaims struct {
	ID    uint
	Email string
	Role  string
}

// CustomClaims is the JWT payload for both access and refresh tokens.
type CustomClaims struct {
	BaseClaims
	TokenType string
	JTI       string
	// Epoch is the user's token epoch at mint time. AuthMiddleware rejects the
	// token if the user's current epoch is higher (force-revocation on
	// role/status change or password reset). Absent in pre-existing tokens →
	// decodes to 0, which stays valid until the user's epoch is first bumped.
	Epoch int64
	gojwt.RegisteredClaims
}

// GetToken extracts the raw Bearer token string from the Authorization header.
// Returns an empty string if the header is absent or malformed.
func GetToken(c *gin.Context) string {
	authHeader := c.Request.Header.Get(authorizationHeader)
	if authHeader == "" {
		log.L().Error("empty authorization header")
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != authorizationScheme || parts[1] == "" {
		log.L().Error("invalid authorization header format")
		return ""
	}
	return parts[1]
}

// GetClaims returns the *CustomClaims for the current request from gin context.
// Claims are always set by AuthMiddleware for private routes, so no JWT re-parse
// is needed here.
func GetClaims(c *gin.Context) (*CustomClaims, error) {
	if raw, ok := c.Get("claims"); ok {
		if claims, ok := raw.(*CustomClaims); ok {
			return claims, nil
		}
	}
	return nil, errors.New("no claims in context")
}

// GetUserID returns the authenticated user's ID from the gin context.
// Returns 0 if claims are unavailable.
func GetUserID(c *gin.Context) uint {
	if raw, ok := c.Get("claims"); ok {
		if claims, ok := raw.(*CustomClaims); ok {
			return claims.BaseClaims.ID
		}
	}
	return 0
}

// GetUserRole returns the authenticated user's role from the gin context claims.
func GetUserRole(c *gin.Context) string {
	if raw, ok := c.Get("claims"); ok {
		if claims, ok := raw.(*CustomClaims); ok {
			return claims.BaseClaims.Role
		}
	}
	return ""
}
