package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	emailpkg "antelope/internal/modules/email"
	"antelope/pkg/response"
	"antelope/pkg/types"
	authsvc "antelope/services/auth"
)

// AuthHandler handles authentication and provider management routes.
type AuthHandler struct {
	svc authsvc.Service
}

func NewAuthHandler(svc authsvc.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// @Summary Local login
// @Description Authenticate a user with email and password
// @Tags auth
// @Produce json
// @Accept json
// @Param body body types.LoginDto true "Login credentials"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /auth/login [post]
func (h *AuthHandler) LocalLogin(c *gin.Context) {
	var req types.LoginDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if !emailpkg.VerifyEmailFormat(req.Email) {
		response.CheckFail(c, nil, response.EmailFormatCheck)
		return
	}
	if len(req.Password) < 6 {
		response.CheckFail(c, nil, response.PasswordCheck)
		return
	}
	data, err := h.svc.LocalLogin(c, req)
	response.Render(c, data, err)
}

// @Summary Refresh access token
// @Description Obtain a new access token using a refresh token
// @Tags auth
// @Produce json
// @Accept json
// @Param body body types.UpdateTokenDto true "Refresh token"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req types.UpdateTokenDto
	_ = c.ShouldBind(&req)

	refreshToken := req.RefreshToken
	if refreshToken == "" {
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			refreshToken = authHeader[7:]
		}
	}
	if refreshToken == "" {
		response.Fail(c, nil, response.Unauthorized)
		return
	}
	data, err := h.svc.RefreshToken(refreshToken)
	response.Render(c, data, err)
}

// @Summary Logout
// @Description Log out the current user and invalidate the session
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	err := h.svc.Logout(c)
	response.Render(c, nil, err)
}

// @Summary LDAP login
// @Description Authenticate a user via LDAP
// @Tags auth
// @Produce json
// @Accept json
// @Param body body types.LdapLoginDto true "LDAP credentials"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /auth/ldap/login [post]
func (h *AuthHandler) LdapLogin(c *gin.Context) {
	var req types.LdapLoginDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.LdapLogin(c, req)
	response.Render(c, data, err)
}

// @Summary Start OIDC login
// @Description Mint a one-time state nonce and return the provider authorization URL to redirect to
// @Tags auth
// @Produce json
// @Param provider path string true "Provider name"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /auth/oidc/{provider}/start [get]
func (h *AuthHandler) OidcStart(c *gin.Context) {
	provider := c.Param("provider")
	data, err := h.svc.StartOidc(c, provider)
	response.Render(c, data, err)
}

// @Summary OIDC callback
// @Description Handle the OIDC provider callback and complete authentication
// @Tags auth
// @Produce json
// @Accept json
// @Param body body types.OidcCallbackDto true "OIDC callback payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /auth/oidc/callback [post]
func (h *AuthHandler) OidcCallback(c *gin.Context) {
	var req types.OidcCallbackDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.OidcCallback(c, req)
	response.Render(c, data, err)
}

// @Summary List enabled auth providers
// @Description Get the list of currently enabled authentication providers
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /auth/providers [get]
func (h *AuthHandler) GetEnabledProviders(c *gin.Context) {
	data, err := h.svc.GetEnabledProviders()
	response.Render(c, data, err)
}

// @Summary List all auth providers
// @Description Get the list of all configured authentication providers
// @Tags auth-providers
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /system/auth/providers [get]
func (h *AuthHandler) GetAllProviders(c *gin.Context) {
	data, err := h.svc.GetAllProviders()
	response.Render(c, data, err)
}

// @Summary Add auth provider
// @Description Add a new authentication provider
// @Tags auth-providers
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.AuthProviderAddDto true "Auth provider config"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /system/auth/provider/add [post]
func (h *AuthHandler) AddProvider(c *gin.Context) {
	var req types.AuthProviderAddDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.AddProvider(req)
	response.Render(c, data, err)
}

// @Summary Update auth provider
// @Description Update an existing authentication provider
// @Tags auth-providers
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.AuthProviderUpdateDto true "Auth provider config"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /system/auth/provider/update [post]
func (h *AuthHandler) UpdateProvider(c *gin.Context) {
	var req types.AuthProviderUpdateDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	response.Render(c, nil, h.svc.UpdateProvider(req))
}

// @Summary Delete auth provider
// @Description Delete an authentication provider by ID
// @Tags auth-providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /system/auth/provider/delete/{id} [delete]
func (h *AuthHandler) DeleteProvider(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	response.Render(c, nil, h.svc.DeleteProvider(uint(id)))
}

// @Summary Test auth provider
// @Description Test the connectivity/configuration of an authentication provider by ID
// @Tags auth-providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /system/auth/provider/test/{id} [post]
func (h *AuthHandler) TestProvider(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.TestProvider(uint(id))
	response.Render(c, data, err)
}
