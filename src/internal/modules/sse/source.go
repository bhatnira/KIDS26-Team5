package sse

import "context"

// SSESource is implemented by any type that can produce server-sent events.
// The sse package remains business-agnostic; concrete implementations live in
// the services layer (e.g. LogSSESource, ChatSSESource).
type SSESource interface {
	// ConnectionID returns a unique key for this connection instance used by
	// the Manager to track and cancel it.
	ConnectionID() string

	// Stream writes SSE events to w until the context is canceled or the
	// source is naturally exhausted. It should return nil on clean shutdown
	// and a non-nil error on failure.
	Stream(ctx context.Context, w *Writer) error
}
