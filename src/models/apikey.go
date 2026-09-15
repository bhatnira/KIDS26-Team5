package models

import (
	"time"

	"gorm.io/gorm"
)

// APIKey is a user-generated programmatic credential. The credential itself is a
// JWT access token signed with the access key; only metadata is persisted here.
// The raw token is shown to the user once at creation and never stored.
//
// JTI links the row to the token's unique ID so revocation can be enforced via
// the session store (the same mechanism logout uses).
type APIKey struct {
	gorm.Model

	UserId    uint      `gorm:"index;not null" json:"user_id"`
	Name      string    `gorm:"type:varchar(128);not null" json:"name"`
	JTI       string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"-"`
	Prefix    string    `gorm:"type:varchar(24)" json:"prefix"` // display hint only, e.g. "eyJhbGciOiJI…"
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`

	User *User `gorm:"foreignKey:UserId;constraint:OnDelete:CASCADE" json:"-"`
}
