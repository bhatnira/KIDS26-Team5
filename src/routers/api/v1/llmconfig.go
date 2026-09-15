package v1

import (
	"github.com/gin-gonic/gin"

	"antelope/pkg/response"
	"antelope/pkg/types"
	authsvc "antelope/services/auth"
	llmcfgsvc "antelope/services/llmconfig"
)

// LLMConfigHandler handles per-user LLM configuration endpoints.
type LLMConfigHandler struct {
	svc llmcfgsvc.Service
}

func NewLLMConfigHandler(svc llmcfgsvc.Service) *LLMConfigHandler {
	return &LLMConfigHandler{svc: svc}
}

// @Summary List LLM configs
// @Description List LLM configs
// @Tags llm-config
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /llm/configs [get]
func (h *LLMConfigHandler) ListConfigs(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.GetConfigs(userID)
	response.Render(c, data, err)
}

// @Summary Add LLM config
// @Description Add LLM config
// @Tags llm-config
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body types.LLMConfigAddDto true "LLM config"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /llm/configs [post]
func (h *LLMConfigHandler) AddConfig(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var req types.LLMConfigAddDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.AddConfig(userID, req)
	response.Render(c, data, err)
}

// @Summary Update LLM config
// @Description Update LLM config
// @Tags llm-config
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Config ID"
// @Param body body types.LLMConfigAddDto true "LLM config"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /llm/configs/{id} [put]
func (h *LLMConfigHandler) UpdateConfig(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	configID := c.Param("id")
	if configID == "" {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var req types.LLMConfigAddDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.UpdateConfig(userID, configID, req)
	response.Render(c, data, err)
}

// @Summary Delete LLM config
// @Description Delete LLM config
// @Tags llm-config
// @Produce json
// @Security BearerAuth
// @Param id path string true "Config ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /llm/configs/{id} [delete]
func (h *LLMConfigHandler) DeleteConfig(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	configID := c.Param("id")
	if configID == "" {
		response.Fail(c, nil, response.RequestError)
		return
	}
	response.Render(c, nil, h.svc.DeleteConfig(userID, configID))
}

// @Summary Test LLM connection
// @Description Test LLM connection
// @Tags llm-config
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body types.LLMTestConnectionDto true "Test connection"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /llm/test-connection [post]
func (h *LLMConfigHandler) TestConnection(c *gin.Context) {
	var req types.LLMTestConnectionDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.TestConnection(req)
	response.Render(c, data, err)
}
