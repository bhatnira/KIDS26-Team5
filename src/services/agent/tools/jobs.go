// Package tools contains the Antelope-specific tool implementations the
// agent registers with every user's LLMAgent. Each tool is constructed
// once at app start-up (typically inside services/agent.NewService) and
// passed to the agent module via FactoryDeps.CustomTools. Tools read the
// requesting user's identity from ctx via agent.UserIDFromContext.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"antelope/internal/modules/agent"
	"antelope/models"
	"antelope/pkg/types"

	"gorm.io/gorm"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// ── get_job_status ──────────────────────────────────────────────────────────

type getJobStatusTool struct {
	db *gorm.DB
}

// NewGetJobStatusTool returns a tool the agent can call to look up a
// single job's status. Results are scoped to the requesting user.
func NewGetJobStatusTool(db *gorm.DB) tool.CallableTool {
	return &getJobStatusTool{db: db}
}

func (t *getJobStatusTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name:        "get_job_status",
		Description: "Get the status of a specific job by its ID. Returns the job's pipeline name/version, status, dispatch and allocation identifiers, creation timestamp, and the input parameters it was submitted with.",
		InputSchema: &tool.Schema{
			Type:     "object",
			Required: []string{"job_id"},
			Properties: map[string]*tool.Schema{
				"job_id": {Type: "integer", Description: "Numeric ID of the job."},
			},
		},
	}
}

type getJobStatusInput struct {
	JobID uint `json:"job_id"`
}

func (t *getJobStatusTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	userID, ok := agent.UserIDFromContext(ctx)
	if !ok {
		return nil, errors.New("user id missing from context")
	}
	var in getJobStatusInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if in.JobID == 0 {
		return nil, errors.New("job_id is required")
	}

	var job models.Job
	err := t.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", in.JobID, userID).
		First(&job).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]any{"error": fmt.Sprintf("job %d not found", in.JobID)}, nil
		}
		return nil, fmt.Errorf("load job: %w", err)
	}
	out := map[string]any{
		"id":               job.ID,
		"pipeline_name":    job.PipelineName,
		"pipeline_version": job.PipelineVersion,
		"status":           job.Status,
		"dispatch_id":      job.DispatchId,
		"alloc_id":         job.AllocId,
		"created_at":       job.CreatedAt,
	}
	if len(job.Params) > 0 {
		out["params"] = job.Params
	}
	return out, nil
}

// ── list_user_jobs ──────────────────────────────────────────────────────────

type listUserJobsTool struct {
	db *gorm.DB
}

// NewListUserJobsTool returns a tool that lists the requesting user's
// most recent jobs (defaulting to 10, capped at 100).
func NewListUserJobsTool(db *gorm.DB) tool.CallableTool {
	return &listUserJobsTool{db: db}
}

func (t *listUserJobsTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name:        "list_user_jobs",
		Description: "List recent jobs for the current user. Returns the job ID, pipeline name/version, status, dispatch id, allocation id, and creation timestamp for each.",
		InputSchema: &tool.Schema{
			Type: "object",
			Properties: map[string]*tool.Schema{
				"limit": {Type: "integer", Description: "Maximum number of jobs to return (default 10, max 100)."},
			},
		},
	}
}

type listUserJobsInput struct {
	Limit int `json:"limit,omitempty"`
}

func (t *listUserJobsTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	userID, ok := agent.UserIDFromContext(ctx)
	if !ok {
		return nil, errors.New("user id missing from context")
	}
	var in listUserJobsInput
	if len(jsonArgs) > 0 {
		if err := json.Unmarshal(jsonArgs, &in); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var jobs []types.JobListDto
	err := t.db.WithContext(ctx).
		Model(&models.Job{}).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Select("id,pipeline_name,pipeline_version,created_at,status,dispatch_id,alloc_id").
		Scan(&jobs).Error
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return jobs, nil
}
