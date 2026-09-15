package models

import (
	"gorm.io/gorm"
)

// AuthSource defines where the user was authenticated from
type AuthSource string

const (
	AuthSourceLocal AuthSource = "local"
	AuthSourceLDAP  AuthSource = "ldap"
	AuthSourceOIDC  AuthSource = "oidc"
)

type User struct {
	gorm.Model
	Name       string `gorm:"type:varchar(64);uniqueIndex;not null" json:"name"`
	Email      string `gorm:"type:varchar(128);uniqueIndex;not null" json:"email"`
	Password   string `gorm:"type:varchar(255)" json:"-"` // Can be empty for LDAP/OIDC users, never send to frontend
	Department string `gorm:"type:varchar(64)" json:"department"`
	Group      string `gorm:"type:varchar(36)" json:"group"`
	Role       string `gorm:"type:varchar(20);default:'user'" json:"role"`
	Status     int    `gorm:"size:1;default:1" json:"status"`

	// Auth source tracking
	AuthSource   AuthSource `gorm:"type:varchar(20);default:'local'" json:"auth_source"`
	AuthProvider string     `gorm:"type:varchar(64)" json:"auth_provider"`      // Name of the provider (for external auth)
	ExternalID   string     `gorm:"type:varchar(256);index" json:"external_id"` // External user ID from provider

	Jobs []Job `gorm:"constraint:OnDelete:SET NULL;" json:"jobs,omitempty"`
}

func (u *User) GetUserId() uint {
	return u.ID
}

func (u *User) GetUserName() string {
	return u.Name
}

func (u *User) GetEmail() string {
	return u.Email
}

func (u *User) BeforeDelete(tx *gorm.DB) error {
	// before user deletion
	return tx.Model(&Job{}).Where("user_id = ?", u.GetUserId()).Updates(map[string]any{
		"user_email": u.GetEmail(),
	}).Error
}
