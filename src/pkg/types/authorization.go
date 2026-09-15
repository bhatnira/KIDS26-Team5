package types

// TokenPairResponse is the JSON body returned by /auth/login and /auth/refresh.
//
//	@Description	Access and refresh token pair issued after successful authentication.
type TokenPairResponse struct {
	// Short-lived JWT for authorizing API requests.
	AccessToken string `json:"accessToken"`
	// Long-lived JWT used to obtain a new token pair via /auth/refresh.
	RefreshToken string `json:"refreshToken"`
	// Seconds until the access token expires.
	ExpiresIn int `json:"expiresIn"`
	// Unix timestamp (ms) when the access token expires.
	AccessExpiresAt int64 `json:"accessExpiresAt"`
	// Unix timestamp (ms) when the refresh token expires.
	RefreshExpiresAt int64 `json:"refreshExpiresAt"`
}

// ErrorResponse is the standard error body returned on failure.
//
//	@Description	Standard error response.
type ErrorResponse struct {
	// Machine-readable error code.
	Code int `json:"code"`
	// Human-readable error message.
	Msg string `json:"msg"`
}
