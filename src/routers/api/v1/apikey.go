package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"antelope/pkg/response"
	"antelope/pkg/types"
	apikeysvc "antelope/services/apikey"
	authsvc "antelope/services/auth"
)

// APIKeyHandler handles personal API-key management routes.
type APIKeyHandler struct {
	svc apikeysvc.Service
}

func NewAPIKeyHandler(svc apikeysvc.Service) *APIKeyHandler {
	return &APIKeyHandler{svc: svc}
}

// Generate creates a new API key for the authenticated user.
// The raw key is returned once in the response and never shown again.
// @Summary Generate API key
// @Description Generate a personal API key (JWT access token). Returned once.
// @Tags api-keys
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.APIKeyGenerateDto true "API key payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/api-keys [post]
func (h *APIKeyHandler) Generate(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var req types.APIKeyGenerateDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.Generate(userID, req)
	response.Render(c, data, err)
}

// List returns the authenticated user's API keys (metadata only).
// @Summary List API keys
// @Description List the authenticated user's API keys (no secret material).
// @Tags api-keys
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/api-keys [get]
func (h *APIKeyHandler) List(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.List(userID)
	response.Render(c, data, err)
}

// Revoke deletes an API key and blocks its underlying token.
// @Summary Revoke API key
// @Description Revoke one of the authenticated user's API keys.
// @Tags api-keys
// @Produce json
// @Security BearerAuth
// @Param id path string true "API key ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/api-keys/{id} [delete]
func (h *APIKeyHandler) Revoke(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	response.Render(c, nil, h.svc.Revoke(c.Request.Context(), userID, uint(id)))
}
