package nomad

import "time"

// ---------------------------------------------------------------------------
// Infrastructure-layer GORM models
//
// These models belong to the nomad infrastructure package, NOT the business
// models package. They carry explicit table names (nomad_infra_* prefix) so
// they can never collide with any business-layer table regardless of what the
// application calls its own tables.
// ---------------------------------------------------------------------------

// infraCheckPoint records the last successfully processed Nomad event index.
// Only a single row (ID = 1) is kept; the monitor upserts it on every commit.
type infraCheckPoint struct {
	ID          uint      `gorm:"primarykey"`
	LastIndex   uint64    `gorm:"not null"`
	LastEventID string    `gorm:"type:varchar(128)"`
	UpdatedAt   time.Time `gorm:"not null"`
}

// TableName pins the table to a namespaced name so it never conflicts with
// business-layer tables.
func (infraCheckPoint) TableName() string {
	return "nomad_infra_checkpoints"
}

// infraProcessedEvent is an idempotency log. Each Nomad event is recorded here
// inside the same DB transaction as the business mutation. If the monitor
// restarts and replays an event, the unique index on event_id causes the
// INSERT to fail, the transaction rolls back, and the event is skipped.
type infraProcessedEvent struct {
	ID          uint      `gorm:"primarykey;autoIncrement"`
	EventID     string    `gorm:"type:varchar(128);uniqueIndex;not null"`
	EventIndex  uint64    `gorm:"not null"`
	EventType   string    `gorm:"type:varchar(64);not null"`
	JobID       string    `gorm:"type:varchar(128);index"`
	ProcessedAt time.Time `gorm:"not null"`
}

func (infraProcessedEvent) TableName() string {
	return "nomad_infra_processed_events"
}
