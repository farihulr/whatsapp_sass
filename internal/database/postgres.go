
package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// NewPostgres membuat koneksi PostgreSQL menggunakan GORM.
func NewPostgres(ctx context.Context, databaseURL string) (*gorm.DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL belum diatur")
	}

	// Membuka koneksi PostgreSQL.
	db, err := gorm.Open(
		postgres.Open(databaseURL),
		&gorm.Config{
			NowFunc: func() time.Time {
				return time.Now().UTC()
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka PostgreSQL: %w", err)
	}

	// Mendapatkan koneksi database standar Go SQL.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("gagal mendapatkan koneksi SQL: %w", err)
	}

	// Mengatur connection pool.
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	// Memastikan PostgreSQL benar-benar bisa diakses.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("gagal terhubung ke PostgreSQL: %w", err)
	}

	return db, nil
}
