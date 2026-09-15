package monitor

import (
	"context"
	"fmt"

	"antelope/internal/modules/log"
	nixnomad "antelope/internal/modules/nomad"
	"antelope/models"

	nomad "github.com/hashicorp/nomad/api"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Handler implements nixnomad.EventHandler for the business layer.
// It subscribes to Nomad Allocation events and updates Job.Status and Job.AllocId
// so the application stays in sync with the Nomad cluster. On each status
// transition it also creates a Notification record (same transaction) and
// publishes a Redis pub/sub signal so connected SSE clients refresh immediately.
type Handler struct {
	rdb redis.UniversalClient
}

// NewEventHandler returns a new business EventHandler.
func NewEventHandler(_ *gorm.DB, rdb redis.UniversalClient) nixnomad.EventHandler {
	return &Handler{rdb: rdb}
}

// Topics returns the Nomad topic subscription map.
func (h *Handler) Topics() map[nomad.Topic][]string {
	return map[nomad.Topic][]string{
		nomad.TopicJob:        {"*"},
		nomad.TopicAllocation: {"*"},
	}
}

// Handle processes a single Nomad event inside the provided DB transaction.
//
// Contract (from nixnomad.EventHandler):
//   - tx is an open *gorm.DB transaction; do NOT call tx.Commit/tx.Rollback.
//   - Return a non-empty jobID to record an idempotency key; return "" for events
//     that have no meaningful job association.
const jobBatchDeregistered = "JobBatchDeregistered"

func (h *Handler) Handle(ctx context.Context, tx *gorm.DB, event *nomad.Event) (jobID string, err error) {
	switch event.Topic {
	case nomad.TopicJob:
		return h.handleJobEvent(tx, event)
	case nomad.TopicAllocation:
		return h.handleAllocEvent(ctx, tx, event)
	default:
		return "", nil
	}
}

// handleJobEvent processes TopicJob events.
func (h *Handler) handleJobEvent(tx *gorm.DB, event *nomad.Event) (string, error) {
	if event.Type != jobBatchDeregistered {
		return "", nil
	}

	ext := nixnomad.NewExtendedEvent(event)
	job, err := ext.SafeJob()
	if err != nil {
		return "", fmt.Errorf("monitor handler: decode job payload: %w", err)
	}
	if job == nil || job.ID == nil {
		return "", nil
	}

	nomadJobID := *job.ID
	if err := tx.Model(&models.Job{}).
		Where("dispatch_id = ?", nomadJobID).
		Update("alloc_id", nil).Error; err != nil {
		return nomadJobID, fmt.Errorf("monitor handler: clear alloc_id for dispatch_id %q: %w", nomadJobID, err)
	}

	log.L().Info("alloc_id cleared after JobBatchDeregistered",
		zap.String("dispatch_id", nomadJobID),
	)
	return nomadJobID, nil
}

// handleAllocEvent processes TopicAllocation events, updates Job.Status / Job.AllocId,
// and creates a Notification for the job owner.
func (h *Handler) handleAllocEvent(ctx context.Context, tx *gorm.DB, event *nomad.Event) (string, error) {
	ext := nixnomad.NewExtendedEvent(event)
	alloc, err := ext.SafeAllocation()
	if err != nil {
		return "", fmt.Errorf("monitor handler: decode allocation payload: %w", err)
	}
	if alloc == nil {
		return "", nil
	}

	jobStatus := mapClientStatus(alloc.ClientStatus)
	if jobStatus == "" {
		return "", nil
	}

	// A deregistered (user-stopped) or evicted allocation is reported by Nomad
	// with DesiredStatus "stop"/"evict" while its ClientStatus still walks
	// through running→complete during teardown. Treat it as a failed job rather
	// than letting those transient states mark the job running or completed.
	if alloc.DesiredStatus == nomad.AllocDesiredStatusStop || alloc.DesiredStatus == nomad.AllocDesiredStatusEvict {
		jobStatus = "failed"
	}

	// Lock the row FOR UPDATE so a concurrent user StopJob (or another monitor
	// instance during a leader handover) serializes against this read-modify-
	// write instead of racing it — without the lock a stale read here could
	// overwrite a user-initiated "stop" with a later teardown event.
	var job models.Job
	result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("dispatch_id = ?", alloc.JobID).First(&job)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			log.L().Debug("allocation event for untracked dispatch_id, skipping",
				zap.String("alloc_job_id", alloc.JobID),
				zap.String("alloc_id", alloc.ID))
			return "", nil
		}
		return "", fmt.Errorf("monitor handler: find job by dispatch_id %q: %w", alloc.JobID, result.Error)
	}

	// Never transition out of a terminal status. This keeps a user-initiated
	// stop (StopJob sets "failed") and any already-final state from being
	// overwritten by later teardown events, and prevents duplicate
	// notifications for the same terminal outcome.
	if isTerminalStatus(job.Status) {
		return alloc.JobID, nil
	}

	// Capture the previous status BEFORE tx.Updates mutates the in-memory
	// struct — GORM's Model(&job).Updates assigns the new field values back
	// onto &job after the SQL UPDATE, so reading job.Status afterwards
	// would always show the new value.
	prevStatus := job.Status

	if err := tx.Model(&job).Updates(models.Job{
		Status:  jobStatus,
		AllocId: alloc.ID,
	}).Error; err != nil {
		return "", fmt.Errorf("monitor handler: update job %d status: %w", job.ID, err)
	}

	log.L().Info("job status updated from Nomad allocation event",
		zap.Uint("job_id", job.ID),
		zap.String("dispatch_id", alloc.JobID),
		zap.String("alloc_id", alloc.ID),
		zap.String("nomad_client_status", alloc.ClientStatus),
		zap.String("job_status", jobStatus),
	)

	if job.UserId != nil && prevStatus != jobStatus {
		notif := buildNotification(*job.UserId, jobStatus, job.PipelineName, job.PipelineVersion)
		if err := tx.Create(&notif).Error; err != nil {
			// Non-fatal: log and continue so the job status update is not rolled back.
			log.L().Warn("failed to create notification", zap.Error(err))
		} else {
			// Publish a wake-up signal to the user's SSE stream.
			// If the transaction rolls back the message is spurious but harmless —
			// the SSE handler always re-queries the DB before sending.
			channel := fmt.Sprintf("notify:user:%d", *job.UserId)
			if pubErr := h.rdb.Publish(ctx, channel, notif.ID).Err(); pubErr != nil {
				log.L().Warn("failed to publish notification signal", zap.Error(pubErr))
			}
		}
	}

	return alloc.JobID, nil
}

// buildNotification constructs the Notification record for a job status transition.
func buildNotification(userId uint, jobStatus, pipelineName, pipelineVersion string) models.Notification {
	type meta struct {
		tagTitle string
		tagType  string
		icon     string
	}
	m := map[string]meta{
		"pending":   {"Pending", "info", "icon-park-outline:time"},
		"running":   {"Running", "info", "icon-park-outline:play-one"},
		"completed": {"Completed", "success", "icon-park-outline:check-one"},
		"failed":    {"Failed", "error", "icon-park-outline:close-one"},
	}[jobStatus]

	title := fmt.Sprintf("Job %s %s", pipelineName, jobStatus)
	return models.Notification{
		UserId:   userId,
		Title:    title,
		Desc:     pipelineVersion,
		Type:     0,
		TagTitle: m.tagTitle,
		TagType:  m.tagType,
		Icon:     m.icon,
	}
}

// isTerminalStatus reports whether a job has reached a final state that should
// not be changed by subsequent allocation events.
func isTerminalStatus(status string) bool {
	switch status {
	case "completed", "failed":
		return true
	default:
		return false
	}
}

// mapClientStatus maps a Nomad allocation ClientStatus to a Job.Status string.
func mapClientStatus(clientStatus string) string {
	switch clientStatus {
	case "pending":
		return "pending"
	case "running":
		return "running"
	case "complete":
		return "completed"
	case "failed", "lost":
		return "failed"
	default:
		return ""
	}
}
