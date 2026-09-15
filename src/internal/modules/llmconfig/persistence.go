package llmconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// llmConfigRecord is the durable, infra-layer record of a user's LLM configs
// (the whole array stored as one row). Redis remains the transactional primary
// for the optimistic-locked read-modify-write; this is a durable mirror so the
// configs survive a Redis flush/eviction. Payload is the JSON array, encrypted
// at rest with AES-256-GCM when a key is configured (Encrypted = true).
type llmConfigRecord struct {
	UserID    uint   `gorm:"primaryKey"`
	Encrypted bool   `gorm:"not null"`
	Payload   string `gorm:"type:text;not null"`
	UpdatedAt time.Time
}

func (llmConfigRecord) TableName() string { return "infra_llm_configs" }

// MigrateSchema creates/updates the LLM config persistence table. Call it from
// the coordinated startup migration (under the same Redis lock as the domain
// models) so concurrent pods don't run DDL simultaneously.
func MigrateSchema(db *gorm.DB) error {
	if err := db.AutoMigrate(&llmConfigRecord{}); err != nil {
		return fmt.Errorf("llmconfig: migrate config table: %w", err)
	}
	return nil
}

// persistToDB upserts the durable record for userID, encrypting at rest when a
// key is configured.
func (m *Manager) persistToDB(userID uint, configJSON []byte) error {
	if m.db == nil {
		return nil
	}
	payload := string(configJSON)
	encrypted := false
	if m.box != nil {
		enc, err := m.box.Encrypt(configJSON)
		if err != nil {
			return fmt.Errorf("llmconfig: encrypt config: %w", err)
		}
		payload, encrypted = enc, true
	}
	rec := llmConfigRecord{UserID: userID, Encrypted: encrypted, Payload: payload}
	return m.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"encrypted", "payload", "updated_at"}),
	}).Create(&rec).Error
}

// loadFromDB returns the durable configs for userID. found is false when no
// record exists.
func (m *Manager) loadFromDB(userID uint) (configs []UserLLMConfig, found bool, err error) {
	if m.db == nil {
		return nil, false, nil
	}
	var rec llmConfigRecord
	dbErr := m.db.Where("user_id = ?", userID).First(&rec).Error
	if errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if dbErr != nil {
		return nil, false, fmt.Errorf("llmconfig: load from DB: %w", dbErr)
	}

	plaintext := []byte(rec.Payload)
	if rec.Encrypted {
		if m.box == nil {
			return nil, false, errors.New("llmconfig: config is encrypted but no encryption key is configured")
		}
		dec, decErr := m.box.Decrypt(rec.Payload)
		if decErr != nil {
			return nil, false, fmt.Errorf("llmconfig: decrypt config: %w", decErr)
		}
		plaintext = dec
	}

	if err := json.Unmarshal(plaintext, &configs); err != nil {
		return nil, false, fmt.Errorf("llmconfig: unmarshal: %w", err)
	}
	return configs, true, nil
}
