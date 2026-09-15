// Package models provides domain model definitions.
// This file contains the bootstrap helpers (Migrate, Seed) that were
// previously embedded in app/db.go and app/init.go.
//
// By moving them here the app package no longer needs to import models,
// breaking the circular dependency risk and keeping infrastructure separate
// from domain concerns.
package models

import (
	"strings"

	"antelope/internal/modules/log"
	"antelope/internal/modules/misc"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Migrate runs AutoMigrate for all domain models.
// Pass this to app.WithMigration in main.
//
//	app.NewApp(cfg, app.WithMigration(models.Migrate))
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&JobTemplate{},
		&Pipeline{},
		&Job{},
		&AuthProvider{},
		&Notification{},
		&MCPConfig{},
		&AgentSkill{},
		&AgentWorkspaceConfig{},
		&APIKey{},
	)
}

// SeedConfig carries the runtime values needed by Seed.
type SeedConfig struct {
	SuperUser         string
	SuperUserPassword string
}

// Seed initialises the bootstrap super-user.
// Pass a closure to app.WithSeed in main:
//
//	app.WithSeed(func(db *gorm.DB, r redis.UniversalClient, sm *storage.ClientManager) {
//	    models.Seed(db, models.SeedConfig{
//	        SuperUser:         cfg.System.SuperUser,
//	        SuperUserPassword: cfg.System.SuperUserPassword,
//	    })
//	})
func Seed(db *gorm.DB, cfg SeedConfig) {
	initSuperUser(db, cfg.SuperUser, cfg.SuperUserPassword)
}

// ── private helpers ────────────────────────────────────────────────────────

// initSuperUser seeds the single bootstrap admin account. The configured
// email is created with the "super" role when absent, or promoted to "super"
// when it already exists with a lesser role.
func initSuperUser(db *gorm.DB, superUser, password string) {
	emailAddr := strings.TrimSpace(superUser)
	if emailAddr == "" {
		log.L().Info("no super user configured")
		return
	}

	if password == "" {
		log.L().Error("super user password is empty; refusing to seed super user")
		return
	}

	var user User
	result := db.Where("email = ?", emailAddr).First(&user)

	switch result.Error {
	case gorm.ErrRecordNotFound:
		newUser := User{
			Name:       strings.Split(emailAddr, "@")[0],
			Email:      emailAddr,
			Password:   misc.BcryptHash(password),
			Role:       "super",
			Status:     1,
			AuthSource: AuthSourceLocal,
		}
		if err := db.Create(&newUser).Error; err != nil {
			log.L().Error("failed to create super user", zap.String("email", emailAddr), zap.Error(err))
			return
		}
		log.L().Info("created super user", zap.String("email", emailAddr))

	case nil:
		if user.Role != "super" {
			if err := db.Model(&user).Update("role", "super").Error; err != nil {
				log.L().Error("failed to update user to super role", zap.String("email", emailAddr), zap.Error(err))
				return
			}
			log.L().Info("updated user to super role", zap.String("email", emailAddr))
		}

	default:
		log.L().Error("failed to query user", zap.String("email", emailAddr), zap.Error(result.Error))
	}
}
