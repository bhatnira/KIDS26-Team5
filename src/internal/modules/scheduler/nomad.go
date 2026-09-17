package scheduler

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"antelope/internal/modules/log"

	nomad "github.com/hashicorp/nomad/api"
	"go.uber.org/zap"
)

// NomadDispatcher implements Scheduler using the HashiCorp Nomad API.
type NomadDispatcher struct {
	client *nomad.Client
}

// NewNomadDispatcher creates a new Nomad dispatcher from an existing client.
func NewNomadDispatcher(client *nomad.Client) *NomadDispatcher {
	return &NomadDispatcher{client: client}
}

// Name returns the scheduler type.
func (n *NomadDispatcher) Name() string {
	return "nomad"
}

// Dispatch submits a parameterized job via Nomad.
func (n *NomadDispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResponse, error) {
	opts := (&nomad.WriteOptions{}).WithContext(ctx)

	resp, _, err := n.client.Jobs().Dispatch(
		req.JobID,
		req.Meta,
		req.Payload,
		req.IDPrefix,
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("nomad dispatch failed: %w", err)
	}

	log.L().Info("nomad job dispatched",
		zap.String("job_id", req.JobID),
		zap.String("dispatch_id", resp.DispatchedJobID))

	return &DispatchResponse{DispatchedID: resp.DispatchedJobID}, nil
}

// Deregister removes a Nomad job.
func (n *NomadDispatcher) Deregister(ctx context.Context, jobID string) error {
	opts := (&nomad.WriteOptions{}).WithContext(ctx)

	_, _, err := n.client.Jobs().Deregister(jobID, false, opts)
	if err != nil {
		// Treat "not found" as success
		if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			log.L().Warn("nomad job not found, treating as deregistered", zap.String("job_id", jobID))
			return nil
		}
		return fmt.Errorf("nomad deregister failed: %w", err)
	}

	log.L().Info("nomad job deregistered", zap.String("job_id", jobID))
	return nil
}

// GetStatus queries the Nomad allocation status.
func (n *NomadDispatcher) GetStatus(ctx context.Context, jobID string) (JobStatus, error) {
	opts := (&nomad.QueryOptions{}).WithContext(ctx)

	// Query allocations for this job
	allocs, _, err := n.client.Jobs().Allocations(jobID, false, opts)
	if err != nil {
		return "", fmt.Errorf("nomad allocations query failed: %w", err)
	}

	if len(allocs) == 0 {
		return JobStatusPending, nil
	}

	// Get the most recent allocation
	alloc := allocs[0]
	return mapNomadStatus(alloc.ClientStatus), nil
}

// mapNomadStatus converts Nomad client status to our JobStatus.
func mapNomadStatus(status string) JobStatus {
	switch strings.ToLower(status) {
	case "pending":
		return JobStatusPending
	case "running":
		return JobStatusRunning
	case "complete":
		return JobStatusCompleted
	case "failed", "lost":
		return JobStatusFailed
	default:
		return JobStatusPending
	}
}

// StreamLogs opens a log stream from a Nomad allocation.
func (n *NomadDispatcher) StreamLogs(ctx context.Context, allocID string, logType LogType) (LogStream, error) {
	// Get allocation info
	opts := (&nomad.QueryOptions{}).WithContext(ctx)
	alloc, _, err := n.client.Allocations().Info(allocID, opts)
	if err != nil {
		return nil, fmt.Errorf("nomad allocation info failed: %w", err)
	}

	// Determine log type string for Nomad
	logTypeStr := "stdout"
	if logType == LogTypeStderr {
		logTypeStr = "stderr"
	}

	// Start streaming logs
	cancelCh := make(chan struct{})
	logCh, errCh := n.client.AllocFS().Logs(
		alloc,
		true,          // follow
		"run",         // task name (matches TaskName constant in templates.go)
		logTypeStr,    // "stdout" or "stderr"
		"start",       // origin
		0,             // offset
		cancelCh,
		nil,
	)

	// Create a pipe to adapt the channel-based API to io.ReadCloser
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-logCh:
				if !ok {
					return
				}
				// Write the log data
				if len(frame.Data) > 0 {
					if _, err := pw.Write(frame.Data); err != nil {
						return
					}
				}
			case err, ok := <-errCh:
				if !ok {
					return
				}
				if err != nil {
					log.L().Warn("nomad log stream error", zap.Error(err))
					return
				}
				// Check if allocation terminated
				if alloc != nil {
					updated, _, err := n.client.Allocations().Info(allocID, (&nomad.QueryOptions{}).WithContext(ctx))
					if err == nil && updated.ClientTerminalStatus() != "" {
						return
					}
				}
			case <-cancelCh:
				return
			}
		}
	}()

	return pr, nil
}

// HealthCheck verifies Nomad is reachable.
func (n *NomadDispatcher) HealthCheck(ctx context.Context) error {
	leader, err := n.client.Status().Leader()
	if err != nil {
		return fmt.Errorf("nomad health check failed: %w", err)
	}

	if leader == "" {
		return fmt.Errorf("nomad has no leader elected")
	}

	return nil
}

// Client returns the underlying Nomad client (needed for backward compatibility).
func (n *NomadDispatcher) Client() *nomad.Client {
	return n.client
}

// Ensure NomadDispatcher satisfies the Scheduler interface at compile time.
var _ Scheduler = (*NomadDispatcher)(nil)

// NomadStreamLogOptions contains options for Nomad log streaming.
type NomadStreamLogOptions struct {
	AllocID  string
	TaskName string
	LogType  string
}

// NomadLogSSESource implements SSESource for Nomad log streaming.
type NomadLogSSESource struct {
	client *nomad.Client
	opts   NomadStreamLogOptions
}

// NewNomadLogSSESource creates a new Nomad log SSE source.
func NewNomadLogSSESource(client *nomad.Client, allocID, taskName, logType string) *NomadLogSSESource {
	return &NomadLogSSESource{
		client: client,
		opts: NomadStreamLogOptions{
			AllocID:  allocID,
			TaskName: taskName,
			LogType:  logType,
		},
	}
}

// ConnectionID returns a unique identifier for this SSE connection.
func (s *NomadLogSSESource) ConnectionID() string {
	return fmt.Sprintf("nomad-log-%s-%s", s.opts.AllocID, s.opts.LogType)
}

// Stream implements the SSESource interface.
func (s *NomadLogSSESource) Stream(ctx context.Context, w interface{ Write([]byte) (int, error) }) error {
	// This would delegate to the existing stream_log.go logic
	// For now, use the Dispatcher's StreamLogs method
	stream, err := s.client.Allocations().Info(s.opts.AllocID, (&nomad.QueryOptions{}).WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to get allocation info: %w", err)
	}

	_ = stream // Would proceed with log streaming logic

	return fmt.Errorf("NomadLogSSESource.Stream not yet implemented - use existing stream_log.go")
}

// Ensure we satisfy any required interfaces
var _ io.Closer = (*NomadDispatcher)(nil)

// Close is a no-op for NomadDispatcher (client cleanup is handled elsewhere).
func (n *NomadDispatcher) Close() error {
	return nil
}

// Timeout returns a context with a reasonable timeout for Nomad operations.
func nomadTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}
