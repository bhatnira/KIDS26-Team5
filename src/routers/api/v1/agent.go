package v1

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	agentmod "antelope/internal/modules/agent"
	"antelope/pkg/response"
	agentsvc "antelope/services/agent"
	authsvc "antelope/services/auth"
)

// AgentHandler exposes the AI agent surface: conversation CRUD,
// streaming chat, workspace/MCP/skills settings, and artifact access.
type AgentHandler struct {
	svc agentsvc.Service
}

func NewAgentHandler(svc agentsvc.Service) *AgentHandler {
	return &AgentHandler{svc: svc}
}

// ── Chat ────────────────────────────────────────────────────────────────────

type conversationCreateDto struct {
	Title string `json:"title"`
}

type sendMessageDto struct {
	Content     string                   `json:"content"`
	Attachments []agentmod.AttachmentRef `json:"attachments,omitempty"`
}

// @Summary Create conversation
// @Description Create conversation
// @Tags chat
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body conversationCreateDto true "Request body"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /chat/conversations [post]
func (h *AgentHandler) CreateConversation(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var req conversationCreateDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.CreateConversation(c.Request.Context(), userID, req.Title)
	response.Render(c, data, err)
}

// @Summary List conversations
// @Description List conversations
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /chat/conversations [get]
func (h *AgentHandler) ListConversations(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	data, err := h.svc.ListConversations(c.Request.Context(), userID, limit, offset)
	response.Render(c, data, err)
}

// @Summary Get conversation
// @Description Get conversation
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Param id path string true "Conversation ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /chat/conversations/{id} [get]
func (h *AgentHandler) GetConversation(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	id := c.Param("id")
	if id == "" {
		response.Fail(c, nil, "invalid conversation id")
		return
	}
	data, err := h.svc.GetConversation(c.Request.Context(), userID, id)
	response.Render(c, data, err)
}

// @Summary Delete conversation
// @Description Delete conversation
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Param id path string true "Conversation ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /chat/conversations/{id} [delete]
func (h *AgentHandler) DeleteConversation(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	id := c.Param("id")
	if id == "" {
		response.Fail(c, nil, "invalid conversation id")
		return
	}
	response.Render(c, nil, h.svc.DeleteConversation(c.Request.Context(), userID, id))
}

// SendMessage is intentionally NOT wrapped in response.Render — it owns
// the HTTP response directly and streams Server-Sent Events.
// @Summary Send message (SSE stream)
// @Description Send message (SSE stream)
// @Tags chat
// @Produce text/event-stream
// @Accept json
// @Security BearerAuth
// @Param id path string true "Conversation ID"
// @Param body body sendMessageDto true "Request body"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /chat/conversations/{id}/messages [post]
func (h *AgentHandler) SendMessage(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	id := c.Param("id")
	if id == "" {
		response.Fail(c, nil, "invalid conversation id")
		return
	}
	var req sendMessageDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	h.svc.SendMessageStream(c, userID, id, req.Content, req.Attachments)
}

// ── Workspace ───────────────────────────────────────────────────────────────

// @Summary Get workspace config
// @Description Get workspace config
// @Tags agent
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/workspace-config [get]
func (h *AgentHandler) GetWorkspaceConfig(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.GetWorkspaceConfig(c.Request.Context(), userID)
	response.Render(c, data, err)
}

// @Summary Update workspace config
// @Description Update workspace config
// @Tags agent
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body agentsvc.WorkspaceConfigDto true "Request body"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/workspace-config [put]
func (h *AgentHandler) UpdateWorkspaceConfig(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var req agentsvc.WorkspaceConfigDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.UpdateWorkspaceConfig(c.Request.Context(), userID, req)
	response.Render(c, data, err)
}

// ── MCP ─────────────────────────────────────────────────────────────────────

// @Summary List MCP configs
// @Description List MCP configs
// @Tags agent
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/mcp-configs [get]
func (h *AgentHandler) ListMCPConfigs(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.ListMCPConfigs(c.Request.Context(), userID)
	response.Render(c, data, err)
}

// @Summary Add MCP config
// @Description Add MCP config
// @Tags agent
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body agentsvc.MCPConfigDto true "Request body"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/mcp-configs [post]
func (h *AgentHandler) AddMCPConfig(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var req agentsvc.MCPConfigDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.AddMCPConfig(c.Request.Context(), userID, req)
	response.Render(c, data, err)
}

// @Summary Update MCP config
// @Description Update MCP config
// @Tags agent
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param id path string true "MCP config ID"
// @Param body body agentsvc.MCPConfigDto true "Request body"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/mcp-configs/{id} [put]
func (h *AgentHandler) UpdateMCPConfig(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, nil, "invalid id")
		return
	}
	var req agentsvc.MCPConfigDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, svcErr := h.svc.UpdateMCPConfig(c.Request.Context(), userID, uint(id), req)
	response.Render(c, data, svcErr)
}

// @Summary Delete MCP config
// @Description Delete MCP config
// @Tags agent
// @Produce json
// @Security BearerAuth
// @Param id path string true "MCP config ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/mcp-configs/{id} [delete]
func (h *AgentHandler) DeleteMCPConfig(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, nil, "invalid id")
		return
	}
	response.Render(c, nil, h.svc.DeleteMCPConfig(c.Request.Context(), userID, uint(id)))
}

// ── Skills ──────────────────────────────────────────────────────────────────

// @Summary List skills
// @Description List the built-in skill library plus the caller's own skills.
// @Description The built-in library is ~700 skills, so results are filtered and
// @Description paged server-side; the response also carries the domain facets and
// @Description the loaded library's provenance.
// @Tags agent
// @Produce json
// @Security BearerAuth
// @Param search query string false "Free-text match on name, description, domain and tags"
// @Param domain query string false "Restrict to one domain"
// @Param scope query string false "Restrict to built-in or user skills" Enums(global, user)
// @Param limit query int false "Page size (default 50, max 200)"
// @Param offset query int false "Page offset"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/skills [get]
func (h *AgentHandler) ListSkills(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	q := agentsvc.SkillQuery{
		Search: strings.TrimSpace(c.Query("search")),
		Domain: strings.TrimSpace(c.Query("domain")),
		Scope:  strings.TrimSpace(c.Query("scope")),
	}
	// Unparseable paging values fall back to the defaults rather than failing
	// the request; there is no useful action the caller could take on an error.
	q.Limit, _ = strconv.Atoi(c.Query("limit"))
	q.Offset, _ = strconv.Atoi(c.Query("offset"))

	data, err := h.svc.ListSkills(c.Request.Context(), userID, q)
	response.Render(c, data, err)
}

// @Summary Get skill
// @Description Get skill
// @Tags agent
// @Produce json
// @Security BearerAuth
// @Param name path string true "Skill name"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/skills/{name} [get]
func (h *AgentHandler) GetSkill(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	name := c.Param("name")
	data, err := h.svc.GetSkill(c.Request.Context(), userID, name)
	response.Render(c, data, err)
}

// ── Artifacts ───────────────────────────────────────────────────────────────

// ListArtifacts: GET /agent/sessions/:id/artifacts
// @Summary List session artifacts
// @Description List session artifacts
// @Tags agent
// @Produce json
// @Security BearerAuth
// @Param id path string true "Session ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/sessions/{id}/artifacts [get]
func (h *AgentHandler) ListArtifacts(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.ListArtifacts(c.Request.Context(), userID, c.Param("id"))
	response.Render(c, data, err)
}

// DownloadArtifact: GET /agent/sessions/:id/artifacts/*key — the wildcard
// matches both "filename" (latest version) and "filename/3" (specific
// version). Query string ?version=N also works.
// @Summary Download session artifact
// @Description Download session artifact
// @Tags agent
// @Produce json
// @Security BearerAuth
// @Param id path string true "Session ID"
// @Param key path string true "Artifact key"
// @Param version query string false "Artifact version"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /agent/sessions/{id}/artifacts/{key} [get]
func (h *AgentHandler) DownloadArtifact(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	sessionID := c.Param("id")
	key := strings.TrimPrefix(c.Param("key"), "/")
	name, version := parseArtifactKey(key, c.Query("version"))
	data, err := h.svc.GetArtifactDownloadURL(c.Request.Context(), userID, sessionID, name, version)
	response.Render(c, data, err)
}

func parseArtifactKey(key, versionQS string) (name string, version int) {
	version = -1
	if versionQS != "" {
		if v, err := strconv.Atoi(versionQS); err == nil {
			version = v
		}
	}
	if key == "" {
		return "", version
	}
	// "filename" or "filename/3" — split on the last "/".
	if idx := strings.LastIndex(key, "/"); idx >= 0 {
		tail := key[idx+1:]
		if v, err := strconv.Atoi(tail); err == nil {
			return key[:idx], v
		}
	}
	return key, version
}
