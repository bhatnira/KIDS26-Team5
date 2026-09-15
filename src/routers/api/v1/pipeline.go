package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"antelope/internal/modules/git"
	"antelope/pkg/response"
	"antelope/pkg/types"
	pipesvc "antelope/services/pipeline"
)

// PipelineHandler handles pipeline CRUD routes.
type PipelineHandler struct {
	svc pipesvc.Service
}

func NewPipelineHandler(svc pipesvc.Service) *PipelineHandler {
	return &PipelineHandler{svc: svc}
}

// @Summary List pipelines
// @Description List pipelines
// @Tags pipelines
// @Produce json
// @Param page query string false "page number"
// @Param page_size query string false "page size"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /pipeline/list [get]
func (h *PipelineHandler) GetList(c *gin.Context) {
	page, errPage := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, errSize := strconv.Atoi(c.Query("page_size"))
	if page <= 0 || pageSize <= 0 || errPage != nil || errSize != nil {
		response.CheckFail(c, nil, response.PageOrSizeError)
		return
	}
	if pageSize > 128 {
		response.CheckFail(c, nil, response.TooManyRequests)
		return
	}
	data, err := h.svc.GetList(page, pageSize)
	response.Render(c, data, err)
}

// @Summary List all pipelines
// @Description List all pipelines
// @Tags pipelines
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /pipeline/list_all [get]
func (h *PipelineHandler) GetAll(c *gin.Context) {
	data, err := h.svc.GetAll()
	response.Render(c, data, err)
}

// @Summary Add pipeline
// @Description Add pipeline
// @Tags pipelines
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body types.PipelineAddDto true "pipeline payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /pipeline/add [post]
func (h *PipelineHandler) Add(c *gin.Context) {
	var req types.PipelineAddDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if exists, _ := git.CheckGitRepoExists(req.Repository, req.Version); !exists {
		response.CheckFail(c, nil, response.PipelineNotExist)
		return
	}
	response.Render(c, nil, h.svc.Add(req))
}

// @Summary Update pipeline
// @Description Update pipeline
// @Tags pipelines
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body types.PipelineAddDto true "pipeline payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /pipeline/update [post]
func (h *PipelineHandler) Update(c *gin.Context) {
	var req types.PipelineAddDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if exists, _ := git.CheckGitRepoExists(req.Repository, req.Version); !exists {
		response.CheckFail(c, nil, response.PipelineNotExist)
		return
	}
	response.Render(c, nil, h.svc.Update(req))
}

// @Summary Delete pipeline
// @Description Delete pipeline
// @Tags pipelines
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body types.PipelineDeleteDto true "pipeline delete payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /pipeline/delete [delete]
func (h *PipelineHandler) Delete(c *gin.Context) {
	var req types.PipelineDeleteDto
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	response.Render(c, nil, h.svc.Delete(req))
}

// @Summary Get pipeline parameter schema
// @Description Get pipeline parameter schema
// @Tags pipelines
// @Produce json
// @Security BearerAuth
// @Param repository query string false "repository"
// @Param version query string false "version"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /pipeline/schema [get]
func (h *PipelineHandler) GetSchema(c *gin.Context) {
	repository := c.Query("repository")
	version := c.Query("version")
	if repository == "" {
		response.CheckFail(c, nil, response.RequestError)
		return
	}
	data, err := h.svc.GetSchema(repository, version)
	response.Render(c, data, err)
}
