package database

import (
	"fmt"

	"whatsapp-saas/internal/models"

	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("koneksi database tidak boleh nil")
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.Membership{},
		&models.Invitation{},
		&models.WhatsAppConnection{},
		&models.Conversation{},
		&models.Message{},
	); err != nil {
		return fmt.Errorf("gagal melakukan auto migration: %w", err)
	}

	return nil
}
