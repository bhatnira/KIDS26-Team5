package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"antelope/models"
	"antelope/pkg/response"
)

// JobTemplateHandler handles job template CRUD routes.
type JobTemplateHandler struct {
	db *gorm.DB
}

func NewJobTemplateHandler(db *gorm.DB) *JobTemplateHandler {
	return &JobTemplateHandler{db: db}
}

// @Summary List job templates
// @Description List job templates
// @Tags templates
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /templates [get]
func (h *JobTemplateHandler) List(c *gin.Context) {
	var templates []models.JobTemplate
	if err := h.db.Select("id, name, engine, version, description, is_built_in, created_at, updated_at").
		Order("engine ASC, name ASC").
		Find(&templates).Error; err != nil {
		response.ServerError(c, nil, response.SystemError)
		return
	}
	response.Success(c, gin.H{"templates": templates}, response.OK)
}

// @Summary Get job template
// @Description Get job template
// @Tags templates
// @Produce json
// @Param id path string true "template id"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /templates/{id} [get]
func (h *JobTemplateHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var tpl models.JobTemplate
	if err := h.db.First(&tpl, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": response.ResultNotFoundCode, "msg": "template not found"})
		return
	}
	response.Success(c, gin.H{"template": tpl}, response.OK)
}

// @Summary Create job template
// @Description Create job template
// @Tags templates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body models.JobTemplate true "job template payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /templates [post]
func (h *JobTemplateHandler) Create(c *gin.Context) {
	var tpl models.JobTemplate
	if err := c.ShouldBindJSON(&tpl); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if err := h.db.Create(&tpl).Error; err != nil {
		response.ServerError(c, nil, response.SystemError)
		return
	}
	response.Success(c, gin.H{"id": tpl.ID}, response.OK)
}

// @Summary Update job template
// @Description Update job template
// @Tags templates
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "template id"
// @Param body body map[string]interface{} true "job template updates"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /templates/{id} [put]
func (h *JobTemplateHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var existing models.JobTemplate
	if err := h.db.First(&existing, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": response.ResultNotFoundCode, "msg": "template not found"})
		return
	}
	if err := h.db.Model(&existing).Updates(updates).Error; err != nil {
		response.ServerError(c, nil, response.SystemError)
		return
	}
	response.Success(c, nil, response.OK)
}

// @Summary Delete job template
// @Description Delete job template
// @Tags templates
// @Produce json
// @Security BearerAuth
// @Param id path string true "template id"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /templates/{id} [delete]
func (h *JobTemplateHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	var existing models.JobTemplate
	if err := h.db.First(&existing, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": response.ResultNotFoundCode, "msg": "template not found"})
		return
	}
	if existing.IsBuiltIn {
		response.CheckFail(c, nil, "cannot delete built-in template")
		return
	}
	if err := h.db.Unscoped().Delete(&existing).Error; err != nil {
		response.ServerError(c, nil, response.SystemError)
		return
	}
	response.Success(c, nil, response.OK)
}
