package scheduler

import (
	"context"
	"io"
)

// JobStatus represents the status of a dispatched job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// DispatchRequest contains the information needed to dispatch a job.
type DispatchRequest struct {
	// JobID is the unique identifier for the job (e.g., "nf-core-testpipeline-1.0.0").
	JobID string

	// Meta contains scheduler-specific metadata (e.g., Nomad dispatch meta).
	Meta map[string]string

	// Payload is the JSON-encoded dispatch payload (repository, revision, params, storage).
	Payload []byte

	// IDPrefix is an optional prefix for the dispatched job ID.
	IDPrefix string
}

// DispatchResponse contains the result of a successful dispatch.
type DispatchResponse struct {
	// DispatchedID is the scheduler-assigned job ID (e.g., Nomad dispatch ID or Slurm job ID).
	DispatchedID string
}

// LogType represents the type of log stream.
type LogType string

const (
	LogTypeStdout LogType = "stdout"
	LogTypeStderr LogType = "stderr"
)

// LogStream is an io.ReadCloser that streams log lines.
type LogStream = io.ReadCloser

// Scheduler is the abstraction layer for job schedulers (Nomad, Slurm, Kubernetes, etc.).
type Scheduler interface {
	// Name returns the scheduler type (e.g., "nomad", "slurm").
	Name() string

	// Dispatch submits a job to the scheduler.
	Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResponse, error)

	// Deregister removes a job from the scheduler.
	Deregister(ctx context.Context, jobID string) error

	// GetStatus returns the current status of a job.
	GetStatus(ctx context.Context, jobID string) (JobStatus, error)

	// StreamLogs opens a log stream for a job.
	// The caller must close the returned LogStream.
	StreamLogs(ctx context.Context, jobID string, logType LogType) (LogStream, error)

	// HealthCheck verifies the scheduler is reachable.
	HealthCheck(ctx context.Context) error
}
