// Package jwt provides HMAC-signed JWT signing and parsing for access and refresh tokens.
//
// Typical usage from the service layer:
//
//	j := jwt.NewJWT(accessKey, refreshKey)
//	tokenStr, err := j.SignAccess(myClaims)
//	claims, err  := jwt.ParseAccess[*auth.CustomClaims](j, tokenStr)
package jwt

import (
	"errors"
	"fmt"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// Sentinel errors returned by ParseAccess / ParseRefresh.
// Callers should match with errors.Is rather than string comparison.
var (
	ErrTokenExpired          = fmt.Errorf("token expired: %w", gojwt.ErrTokenExpired)
	ErrTokenNotValidYet      = fmt.Errorf("token not valid yet: %w", gojwt.ErrTokenNotValidYet)
	ErrTokenMalformed        = fmt.Errorf("token malformed: %w", gojwt.ErrTokenMalformed)
	ErrTokenSignatureInvalid = fmt.Errorf("token signature invalid: %w", gojwt.ErrTokenSignatureInvalid)
	ErrTokenUnknown          = errors.New("token unknown error")
)

// JWT holds the HMAC signing keys for access and refresh tokens.
type JWT struct {
	AccessSigningKey  []byte
	RefreshSigningKey []byte
}

// NewJWT constructs a JWT signer/parser from the given HMAC signing keys.
func NewJWT(accessSigningKey, refreshSigningKey string) *JWT {
	return &JWT{
		AccessSigningKey:  []byte(accessSigningKey),
		RefreshSigningKey: []byte(refreshSigningKey),
	}
}

// SignAccess signs claims with the access signing key using HS256.
func (j *JWT) SignAccess(claims gojwt.Claims) (string, error) {
	return gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims).SignedString(j.AccessSigningKey)
}

// SignRefresh signs claims with the refresh signing key using HS256.
func (j *JWT) SignRefresh(claims gojwt.Claims) (string, error) {
	return gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims).SignedString(j.RefreshSigningKey)
}

// ParseAccess parses and validates a token signed with the access key,
// deserializing the payload into C.
//
// C must be a pointer to a struct that implements gojwt.Claims, e.g.:
//
//	claims, err := jwt.ParseAccess[auth.CustomClaims, *auth.CustomClaims](j, tokenStr)
func ParseAccess[T any, C interface {
	*T
	gojwt.Claims
}](j *JWT, tokenString string) (C, error) {
	return parse[T, C](tokenString, j.AccessSigningKey)
}

// ParseRefresh parses and validates a token signed with the refresh key.
// See ParseAccess for usage.
func ParseRefresh[T any, C interface {
	*T
	gojwt.Claims
}](j *JWT, tokenString string) (C, error) {
	return parse[T, C](tokenString, j.RefreshSigningKey)
}

// parse is the shared implementation for both access and refresh parsing.
//
// The two-parameter constraint [T any, C interface{ *T; gojwt.Claims }] is the
// standard Go pattern for "pointer to T that also satisfies an interface":
//   - T is the concrete struct (e.g. auth.CustomClaims)
//   - C is the pointer type (e.g. *auth.CustomClaims)
//
// This lets us call new(T) to allocate a non-nil value for ParseWithClaims
// — fixing the nil-pointer bug from the original var zero C approach —
// while still returning the typed pointer C to the caller.
// No reflection is needed.
func parse[T any, C interface {
	*T
	gojwt.Claims
}](tokenString string, key []byte) (C, error) {
	// Allocate a concrete *T so ParseWithClaims has a non-nil target to
	// deserialize into. Casting via C preserves the interface constraint.
	claims := C(new(T))

	token, err := gojwt.ParseWithClaims(tokenString, claims, func(token *gojwt.Token) (any, error) {
		// Validate the algorithm before returning the key.
		// This prevents algorithm-confusion attacks (e.g. alg:none, RS256 swap).
		if _, ok := token.Method.(*gojwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return key, nil
	})
	if err != nil {
		return nil, mapTokenError(err)
	}

	if token != nil {
		if c, ok := token.Claims.(C); ok && token.Valid {
			return c, nil
		}
	}
	return nil, ErrTokenUnknown
}

// mapTokenError translates golang-jwt sentinel errors into the package-level
// wrapped sentinels so callers can use errors.Is for fine-grained matching.
func mapTokenError(err error) error {
	switch {
	case errors.Is(err, gojwt.ErrTokenExpired):
		return ErrTokenExpired
	case errors.Is(err, gojwt.ErrTokenMalformed):
		return ErrTokenMalformed
	case errors.Is(err, gojwt.ErrTokenSignatureInvalid):
		return ErrTokenSignatureInvalid
	case errors.Is(err, gojwt.ErrTokenNotValidYet):
		return ErrTokenNotValidYet
	default:
		return ErrTokenUnknown
	}
}
