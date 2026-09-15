package job

import (
	"context"
	"fmt"
	"net/http"
	"time"

	nomad "github.com/hashicorp/nomad/api"
	"go.uber.org/zap"

	"antelope/internal/modules/log"
	"antelope/internal/modules/sse"
)

// StreamLogOptions holds the parameters that identify a specific Nomad log stream.
type StreamLogOptions struct {
	AllocId  string
	TaskName string
	LogType  string // "stdout" or "stderr"
}

// ConnectionID returns a unique key for this log stream instance.
func (opt *StreamLogOptions) ConnectionID() string {
	return fmt.Sprintf("%s-%s-%s-%d", opt.AllocId, opt.TaskName, opt.LogType, time.Now().UnixNano())
}

// LogSSESource implements sse.SSESource for Nomad allocation log streaming.
type LogSSESource struct {
	nomadClient *nomad.Client
	opts        *StreamLogOptions
	connID      string
}

// NewLogSSESource creates a LogSSESource. ConnectionID is fixed at creation time
// so that the same ID is used throughout the lifetime of the request.
func NewLogSSESource(nomadClient *nomad.Client, opts *StreamLogOptions) *LogSSESource {
	return &LogSSESource{
		nomadClient: nomadClient,
		opts:        opts,
		connID:      opts.ConnectionID(),
	}
}

// ConnectionID satisfies sse.SSESource.
func (s *LogSSESource) ConnectionID() string {
	return s.connID
}

// Stream satisfies sse.SSESource. It fetches the Nomad allocation, opens the
// log channel, and writes SSE events to w until ctx is canceled or the
// allocation terminates.
func (s *LogSSESource) Stream(ctx context.Context, w *sse.Writer) error {
	alloc, _, err := s.nomadClient.Allocations().Info(s.opts.AllocId, nil)
	if err != nil {
		_ = w.WriteDataJSON(map[string]any{
			"type":      "error",
			"error":     fmt.Sprintf("get allocation info failed: %v", err),
			"timestamp": time.Now().Unix(),
		})
		log.L().Error("get allocation info failed",
			zap.String("alloc_id", s.opts.AllocId),
			zap.Error(err))
		return err
	}

	cancelCh := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(cancelCh)
		log.L().Info("close connection with nomad", zap.String("connectionId", s.connID))
	}()

	logCh, errCh := s.nomadClient.AllocFS().Logs(
		alloc,
		true,
		s.opts.TaskName,
		s.opts.LogType,
		"start",
		0,
		cancelCh,
		nil,
	)

	statusTicker := time.NewTicker(5 * time.Second)
	defer statusTicker.Stop()

	_ = w.WriteDataJSON(map[string]any{
		"type":      "connected",
		"message":   fmt.Sprintf("Log stream started (Id: %s)", s.connID),
		"timestamp": time.Now().Unix(),
	})

	for {
		select {
		case <-ctx.Done():
			log.L().Info("context cancelled for connection", zap.String("connectionId", s.connID))
			return nil

		case frame, ok := <-logCh:
			if !ok {
				_ = w.WriteDataJSON(map[string]any{
					"type":      "stream_ended",
					"message":   "log stream ended",
					"timestamp": time.Now().Unix(),
				})
				return nil
			}
			if frame != nil && len(frame.Data) > 0 {
				_ = w.WriteDataJSON(map[string]any{
					"type":      "log",
					"data":      string(frame.Data),
					"timestamp": time.Now().Unix(),
				})
			}

		case streamErr, ok := <-errCh:
			if !ok {
				return nil
			}
			if streamErr != nil {
				_ = w.WriteDataJSON(map[string]any{
					"type":      "error",
					"error":     streamErr.Error(),
					"timestamp": time.Now().Unix(),
				})
				return streamErr
			}

		case <-statusTicker.C:
			currentAlloc, _, allocErr := s.nomadClient.Allocations().Info(s.opts.AllocId, nil)
			if allocErr != nil {
				log.L().Error("check allocation status failed", zap.Error(allocErr))
				continue
			}

			if currentAlloc.ClientTerminalStatus() {
				_ = w.WriteDataJSON(map[string]any{
					"type":      "allocation_terminated",
					"message":   fmt.Sprintf("allocation status: %s", currentAlloc.ClientStatus),
					"timestamp": time.Now().Unix(),
				})

				time.Sleep(500 * time.Millisecond)

				select {
				case frame := <-logCh:
					if frame != nil && len(frame.Data) > 0 {
						_ = w.WriteDataJSON(map[string]any{
							"type":      "log",
							"data":      string(frame.Data),
							"timestamp": time.Now().Unix(),
						})
					}
				default:
				}

				_ = w.WriteDataJSON(map[string]any{
					"type":      "stream_ended",
					"message":   "allocation terminated",
					"timestamp": time.Now().Unix(),
				})
				return nil
			}
		}
	}
}

// StreamLogs writes Nomad allocation logs as SSE events.
// GO-3: accepts context.Context and http.ResponseWriter directly so the service
// layer does not depend on *gin.Context.
func StreamLogs(ctx context.Context, w http.ResponseWriter, nomadClient *nomad.Client, sseMgr *sse.Manager, opts *StreamLogOptions) {
	sseW, err := sse.NewWriter(w)
	if err != nil {
		log.L().Error("sse writer init failed", zap.Error(err))
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	src := NewLogSSESource(nomadClient, opts)
	if err := sseMgr.Serve(ctx, sseW, src); err != nil {
		log.L().Error("log stream ended with error",
			zap.String("connectionId", src.ConnectionID()),
			zap.Error(err))
	}
}
