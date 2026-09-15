package v1

import (
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"

	"antelope/pkg/response"
	"antelope/pkg/types"
	authsvc "antelope/services/auth"
	jobsvc "antelope/services/job"
)

// JobHandler handles job submission and management routes.
type JobHandler struct {
	svc jobsvc.Service
}

func NewJobHandler(svc jobsvc.Service) *JobHandler {
	return &JobHandler{svc: svc}
}

// @Summary Submit job
// @Description Submit a new Nextflow job for the current user
// @Tags jobs
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.JobAddDto true "Job submission payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /job/add [post]
func (h *JobHandler) AddJob(c *gin.Context) {
	var req types.JobAddDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if req.UserId = authsvc.GetUserID(c); req.UserId == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if !json.Valid(req.PipelineParams) {
		response.CheckFail(c, nil, "invalid pipeline parameter format")
		return
	}
	response.Render(c, nil, h.svc.AddJob(req))
}

// @Summary Delete job
// @Description Delete a job owned by the current user
// @Tags jobs
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.JobDeleteDto true "Job delete payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /job/delete [delete]
func (h *JobHandler) DeleteJob(c *gin.Context) {
	var req types.JobDeleteDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if req.UserId = authsvc.GetUserID(c); req.UserId == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if !h.svc.CheckJobOwnership(req.UserId, req.JobId) {
		response.Fail(c, nil, response.Unauthorized)
		return
	}
	response.Render(c, nil, h.svc.DeleteJob(req))
}

// @Summary Stop job
// @Description Stop a running job owned by the current user
// @Tags jobs
// @Produce json
// @Accept json
// @Security BearerAuth
// @Param body body types.JobStopDto true "Job stop payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /job/stop [post]
func (h *JobHandler) StopJob(c *gin.Context) {
	var req types.JobStopDto
	if err := c.ShouldBind(&req); err != nil {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if req.UserId = authsvc.GetUserID(c); req.UserId == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if !h.svc.CheckJobOwnership(req.UserId, req.JobId) {
		response.Fail(c, nil, response.Unauthorized)
		return
	}
	response.Render(c, nil, h.svc.StopJob(req))
}

// @Summary Get job details
// @Description Get details of a job owned by the current user
// @Tags jobs
// @Produce json
// @Security BearerAuth
// @Param job_id query int true "Job ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /job/details [get]
func (h *JobHandler) GetJobDetails(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	jobID, err := strconv.Atoi(c.Query("job_id"))
	if err != nil || jobID <= 0 {
		response.CheckFail(c, nil, response.RequestError)
		return
	}
	if !h.svc.CheckJobOwnership(userID, uint(jobID)) {
		response.Fail(c, nil, response.Unauthorized)
		return
	}
	data, err := h.svc.GetJobDetails(uint(jobID))
	response.Render(c, gin.H{"job": data}, err)
}

// @Summary List current user's jobs
// @Description List jobs for the current user with pagination
// @Tags jobs
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/jobs [get]
func (h *JobHandler) GetUserJobs(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
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
	data, err := h.svc.GetUserJobs(userID, page, pageSize)
	response.Render(c, data, err)
}

// @Summary Stream job log via SSE
// @Description Stream a job's log over Server-Sent Events
// @Tags jobs
// @Produce text/event-stream
// @Security BearerAuth
// @Param allocId query string true "Nomad allocation ID"
// @Param logType query string false "Log type (stdout or stderr)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/job/log [get]
func (h *JobHandler) StreamUserJobLog(c *gin.Context) {
	allocID := c.Query("allocId")
	logType := c.DefaultQuery("logType", "stdout")
	if allocID == "" {
		response.CheckFail(c, nil, response.RequestError)
		return
	}
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if !h.svc.CheckAllocOwnership(userID, allocID) {
		response.CheckFail(c, nil, response.Unauthorized)
		return
	}
	// GO-3: pass context and ResponseWriter directly, not *gin.Context
	h.svc.StreamJobLog(c.Request.Context(), c.Writer, allocID, logType)
}
