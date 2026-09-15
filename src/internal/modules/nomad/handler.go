package nomad

import (
	"context"

	nomad "github.com/hashicorp/nomad/api"
	"gorm.io/gorm"
)

// EventHandler is the contract that business-layer components must satisfy in
// order to receive Nomad events from the EventMonitor.
//
// The EventMonitor guarantees:
//   - At-most-once delivery per event (idempotency key stored in processed_events).
//   - Handle is always called inside an open *gorm.DB transaction; the monitor
//     commits or rolls back that transaction — the handler must NOT call
//     tx.Commit / tx.Rollback itself.
//   - Handle is only called while the current node holds the leader lock.
//
// The handler is responsible for:
//   - All domain / business logic.
//   - Returning a stable, non-empty jobID string that the monitor uses as the
//     idempotency key stored alongside the event record. Return an empty string
//     when the event carries no meaningful job association.
type EventHandler interface {
	// Topics returns the Nomad topic subscription map that the monitor should
	// open an event stream for. Returning nil subscribes to nothing.
	Topics() map[nomad.Topic][]string

	// Handle processes a single Nomad event inside the provided transaction.
	// It returns a jobID (maybe empty) used for idempotency tracking, and any
	// error that should cause the transaction to be rolled back.
	Handle(ctx context.Context, tx *gorm.DB, event *nomad.Event) (jobID string, err error)
}
