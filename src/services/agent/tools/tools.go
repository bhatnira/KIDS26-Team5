package tools

import (
	"antelope/internal/modules/storage"

	"gorm.io/gorm"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// Deps bundles the dependencies needed to construct every Antelope tool.
type Deps struct {
	DB      *gorm.DB
	Storage *storage.ClientManager
	Jobs    JobSubmitter
	Schemas SchemaGetter
	Dash    UserStatsGetter
	Notifs  NotificationLister

	// CabBaseURL is the external CAB (nightingale) upstream base URL
	// (cab.base-url / ANTELOPE_CAB_BASE_URL). Empty falls back to the built-in
	// default in the CAB tool constructors.
	CabBaseURL string
}

// NewBuiltinTools constructs the full set of Antelope-specific tools the
// agent exposes by default. They cover the platform's non-destructive APIs so
// the built-in agent can drive the same read/dispatch flows the website does:
//
//	Pipelines & jobs : list_pipelines, get_pipeline_schema, list_job_templates,
//	                   submit_job, get_job_status, list_user_jobs
//	Storage          : browse_storage, get_download_url
//	Account          : get_dashboard_stats, list_notifications
//	External (CAB)   : query_cab_fastq, submit_cab_pipeline
//
// Destructive operations (delete/stop job, delete pipeline/bucket/object,
// user and config management) are intentionally NOT exposed. Hand the result
// to agent.ManagerDeps.CustomTools when building the agent runner.
func NewBuiltinTools(d Deps) []tool.Tool {
	return []tool.Tool{
		// Pipelines & jobs.
		NewListPipelinesTool(d.DB),
		NewGetPipelineSchemaTool(d.DB, d.Schemas),
		NewListJobTemplatesTool(d.DB),
		NewSubmitJobTool(d.Jobs),
		NewGetJobStatusTool(d.DB),
		NewListUserJobsTool(d.DB),
		// Storage.
		NewBrowseStorageTool(d.Storage),
		NewGetDownloadURLTool(d.Storage),
		// Account.
		NewGetDashboardStatsTool(d.Dash),
		NewListNotificationsTool(d.Notifs),
		// External CAB (nightingale) integration.
		NewQueryCabFastqTool(d.CabBaseURL),
		NewSubmitCabPipelineTool(d.CabBaseURL),
	}
}
