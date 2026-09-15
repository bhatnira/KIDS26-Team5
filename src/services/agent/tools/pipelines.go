package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"antelope/internal/modules/agent"
	"antelope/models"
	"antelope/pkg/apperr"
	"antelope/pkg/types"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// ── list_pipelines ──────────────────────────────────────────────────────────

type listPipelinesTool struct {
	db *gorm.DB
}

// NewListPipelinesTool returns a tool that lists every ready Nextflow
// pipeline available on the platform. The list is global, not per-user.
func NewListPipelinesTool(db *gorm.DB) tool.CallableTool {
	return &listPipelinesTool{db: db}
}

func (t *listPipelinesTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name:        "list_pipelines",
		Description: "List all available Nextflow pipelines with their names, versions, descriptions, authors, and status.",
		InputSchema: &tool.Schema{
			Type:       "object",
			Properties: map[string]*tool.Schema{},
		},
	}
}

type pipelineInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Status      string `json:"status"`
	Repository  string `json:"repository"`
}

func (t *listPipelinesTool) Call(ctx context.Context, _ []byte) (any, error) {
	var pipelines []models.Pipeline
	if err := t.db.WithContext(ctx).
		Where("status = ?", "ready").
		Find(&pipelines).Error; err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}
	out := make([]pipelineInfo, 0, len(pipelines))
	for _, p := range pipelines {
		out = append(out, pipelineInfo{
			Name:        p.Name,
			Version:     p.Version,
			Description: p.Description,
			Author:      p.Author,
			Status:      p.Status,
			Repository:  p.Repository,
		})
	}
	return out, nil
}

// ── submit_job ──────────────────────────────────────────────────────────────

// JobSubmitter is the narrow contract the submit_job tool needs from the
// job service. Declared here (rather than importing services/job) so tools/
// only depends on what it actually uses.
type JobSubmitter interface {
	AddJob(dto types.JobAddDto) error
}

type submitJobTool struct {
	jobs JobSubmitter
}

// NewSubmitJobTool returns the submit_job tool. The caller injects a
// JobSubmitter implementation (typically the existing services/job
// service) so the tool only depends on the AddJob method.
func NewSubmitJobTool(jobs JobSubmitter) tool.CallableTool {
	return &submitJobTool{jobs: jobs}
}

func (t *submitJobTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name:        "submit_job",
		Description: "Submit a Nextflow bioinformatics job. Only call this AFTER the user has confirmed the job configuration.",
		InputSchema: &tool.Schema{
			Type:     "object",
			Required: []string{"pipeline_name", "pipeline_version", "pipeline_params"},
			Properties: map[string]*tool.Schema{
				"pipeline_name":    {Type: "string", Description: "Name of the pipeline to run."},
				"pipeline_version": {Type: "string", Description: "Version of the pipeline."},
				"pipeline_params": {
					Type:                 "object",
					Description:          "Pipeline parameters as key-value pairs.",
					AdditionalProperties: true,
				},
			},
		},
	}
}

type submitJobInput struct {
	PipelineName    string          `json:"pipeline_name"`
	PipelineVersion string          `json:"pipeline_version"`
	PipelineParams  json.RawMessage `json:"pipeline_params"`
}

func (t *submitJobTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	userID, ok := agent.UserIDFromContext(ctx)
	if !ok {
		return nil, errors.New("user id missing from context")
	}
	var in submitJobInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if in.PipelineName == "" || in.PipelineVersion == "" {
		return nil, errors.New("pipeline_name and pipeline_version are required")
	}
	if len(in.PipelineParams) == 0 || string(in.PipelineParams) == "null" {
		return nil, errors.New("pipeline_params is required")
	}

	dto := types.JobAddDto{
		UserId:          userID,
		PipelineName:    in.PipelineName,
		PipelineVersion: in.PipelineVersion,
		PipelineParams:  in.PipelineParams,
	}
	if err := t.jobs.AddJob(dto); err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			return map[string]any{"error": appErr.Msg}, nil
		}
		return map[string]any{"error": err.Error()}, nil
	}
	return map[string]any{"status": "submitted"}, nil
}

// ── get_pipeline_schema ─────────────────────────────────────────────────────

// SchemaGetter is the narrow contract the get_pipeline_schema tool needs
// from the pipeline service (mirrors the /pipeline/schema handler).
type SchemaGetter interface {
	GetSchema(repository, version string) (gin.H, error)
}

type getPipelineSchemaTool struct {
	db      *gorm.DB
	schemas SchemaGetter
}

// NewGetPipelineSchemaTool returns a tool that fetches a pipeline's
// nextflow_schema.json (parameter schema). The pipeline is resolved by
// name(+version) so the agent can chain it directly after list_pipelines.
func NewGetPipelineSchemaTool(db *gorm.DB, schemas SchemaGetter) tool.CallableTool {
	return &getPipelineSchemaTool{db: db, schemas: schemas}
}

func (t *getPipelineSchemaTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name: "get_pipeline_schema",
		Description: "Get the parameter schema (nextflow_schema.json) for a pipeline. " +
			"Call this before submit_job to learn which pipeline_params the pipeline accepts.",
		InputSchema: &tool.Schema{
			Type:     "object",
			Required: []string{"pipeline_name"},
			Properties: map[string]*tool.Schema{
				"pipeline_name":    {Type: "string", Description: "Pipeline name as returned by list_pipelines."},
				"pipeline_version": {Type: "string", Description: "Pipeline version. Defaults to the registered version when omitted."},
			},
		},
	}
}

type getPipelineSchemaInput struct {
	PipelineName    string `json:"pipeline_name"`
	PipelineVersion string `json:"pipeline_version,omitempty"`
}

func (t *getPipelineSchemaTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in getPipelineSchemaInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if in.PipelineName == "" {
		return nil, errors.New("pipeline_name is required")
	}

	q := t.db.WithContext(ctx).Where("name = ?", in.PipelineName)
	if in.PipelineVersion != "" {
		q = q.Where("version = ?", in.PipelineVersion)
	}
	var p models.Pipeline
	if err := q.First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]any{"error": fmt.Sprintf("pipeline %q not found", in.PipelineName)}, nil
		}
		return nil, fmt.Errorf("load pipeline: %w", err)
	}

	version := in.PipelineVersion
	if version == "" {
		version = p.Version
	}
	data, err := t.schemas.GetSchema(p.Repository, version)
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			return map[string]any{"error": appErr.Msg}, nil
		}
		return map[string]any{"error": err.Error()}, nil
	}
	return data, nil
}

// ── list_job_templates ──────────────────────────────────────────────────────

type listJobTemplatesTool struct {
	db *gorm.DB
}

// NewListJobTemplatesTool returns a tool that lists the platform's job
// templates (metadata only — the Nomad HCL body is an admin concern and is
// deliberately not exposed to the agent).
func NewListJobTemplatesTool(db *gorm.DB) tool.CallableTool {
	return &listJobTemplatesTool{db: db}
}

func (t *listJobTemplatesTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name:        "list_job_templates",
		Description: "List the platform's job templates (execution engines a job can run on), with name, engine, version, and description.",
		InputSchema: &tool.Schema{
			Type:       "object",
			Properties: map[string]*tool.Schema{},
		},
	}
}

type jobTemplateInfo struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Engine      string `json:"engine"`
	Version     string `json:"version"`
	Description string `json:"description"`
	IsBuiltIn   bool   `json:"is_built_in"`
}

func (t *listJobTemplatesTool) Call(ctx context.Context, _ []byte) (any, error) {
	var templates []models.JobTemplate
	if err := t.db.WithContext(ctx).
		Select("id, name, engine, version, description, is_built_in").
		Order("engine ASC, name ASC").
		Find(&templates).Error; err != nil {
		return nil, fmt.Errorf("list job templates: %w", err)
	}
	out := make([]jobTemplateInfo, 0, len(templates))
	for _, tpl := range templates {
		out = append(out, jobTemplateInfo{
			ID:          tpl.ID,
			Name:        tpl.Name,
			Engine:      tpl.Engine,
			Version:     tpl.Version,
			Description: tpl.Description,
			IsBuiltIn:   tpl.IsBuiltIn,
		})
	}
	return out, nil
}
