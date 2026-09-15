package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// storageConfigRecord is the durable, infra-layer record of a user's storage
// provider config. Postgres is the source of truth; Redis is a rebuildable cache
// in front of it. Defined inside this module (not the business models package)
// so the module keeps its layering — mirroring the nomad infra-model pattern.
//
// Payload holds the ProviderConfig JSON, encrypted with AES-256-GCM when an
// encryption key is configured (Encrypted = true); otherwise it is stored as
// plaintext JSON (durable but not encrypted).
type storageConfigRecord struct {
	UserID    uint   `gorm:"primaryKey"`
	Type      string `gorm:"type:varchar(32);not null"`
	Hash      string `gorm:"type:varchar(128);not null"`
	Encrypted bool   `gorm:"not null"`
	Payload   string `gorm:"type:text;not null"`
	UpdatedAt time.Time
}

func (storageConfigRecord) TableName() string { return "infra_storage_configs" }

// MigrateSchema creates/updates the storage persistence table. Call it from the
// coordinated startup migration (under the same Redis lock as the domain models)
// so concurrent pods don't run DDL simultaneously.
func MigrateSchema(db *gorm.DB) error {
	if err := db.AutoMigrate(&storageConfigRecord{}); err != nil {
		return fmt.Errorf("storage: migrate config table: %w", err)
	}
	return nil
}

// saveToDB upserts the durable record for userID. configJSON is the marshalled
// ProviderConfig; it is encrypted at rest when a key is configured.
func (m *ClientManager) saveToDB(userID uint, cfg ProviderConfig, configJSON []byte) error {
	if m.db == nil {
		return nil // no durable store wired (e.g. minimal/test setup)
	}

	payload := string(configJSON)
	encrypted := false
	if m.box != nil {
		enc, err := m.box.Encrypt(configJSON)
		if err != nil {
			return fmt.Errorf("storage: encrypt config: %w", err)
		}
		payload, encrypted = enc, true
	}

	rec := storageConfigRecord{
		UserID:    userID,
		Type:      string(cfg.Type),
		Hash:      cfg.Hash,
		Encrypted: encrypted,
		Payload:   payload,
	}
	// Upsert on the user PK.
	return m.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"type", "hash", "encrypted", "payload", "updated_at"}),
	}).Create(&rec).Error
}

// loadFromDB returns the durable ProviderConfig for userID, or (nil, nil) when
// none exists. It decrypts the payload when it was stored encrypted.
func (m *ClientManager) loadFromDB(userID uint) (*ProviderConfig, error) {
	if m.db == nil {
		return nil, nil
	}

	var rec storageConfigRecord
	err := m.db.Where("user_id = ?", userID).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: load config from DB: %w", err)
	}

	plaintext := []byte(rec.Payload)
	if rec.Encrypted {
		if m.box == nil {
			return nil, errors.New("storage: config is encrypted but no encryption key is configured")
		}
		dec, decErr := m.box.Decrypt(rec.Payload)
		if decErr != nil {
			return nil, fmt.Errorf("storage: decrypt config: %w", decErr)
		}
		plaintext = dec
	}

	var cfg ProviderConfig
	if err := json.Unmarshal(plaintext, &cfg); err != nil {
		return nil, fmt.Errorf("storage: deserialize config: %w", err)
	}
	return &cfg, nil
}

// deleteFromDB removes the durable record for userID (no-op if absent).
func (m *ClientManager) deleteFromDB(userID uint) error {
	if m.db == nil {
		return nil
	}
	return m.db.Where("user_id = ?", userID).Delete(&storageConfigRecord{}).Error
}

// existsInDB reports whether a durable record exists for userID.
func (m *ClientManager) existsInDB(userID uint) bool {
	if m.db == nil {
		return false
	}
	var count int64
	if err := m.db.Model(&storageConfigRecord{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}
