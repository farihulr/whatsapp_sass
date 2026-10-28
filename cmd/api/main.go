
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"whatsapp-saas/internal/config"
	"whatsapp-saas/internal/database"
	redisClient "whatsapp-saas/internal/redis"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Gagal memuat konfigurasi: ", err)
	}

	ctx := context.Background()

	// Connect PostgreSQL using GORM
	db, err := database.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Gagal terhubung ke PostgreSQL: ", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("Gagal mendapatkan koneksi SQL: ", err)
	}
	defer sqlDB.Close()

	log.Println("PostgreSQL connected")

	// Automatically migrate GORM models
	if err := database.AutoMigrate(db); err != nil {
		log.Fatal("Gagal melakukan database migration: ", err)
	}

	log.Println("Database migration completed")

	// Connect Redis
	rdb, err := redisClient.NewRedis(
		ctx,
		cfg.RedisAddr,
		cfg.RedisPassword,
		cfg.RedisDB,
	)
	if err != nil {
		log.Fatal("Gagal terhubung ke Redis: ", err)
	}
	defer rdb.Close()

	log.Println("Redis connected")

	// Setup HTTP router
	mux := http.NewServeMux()

	// Liveness check
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	// Readiness check
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(
			r.Context(),
			2*time.Second,
		)
		defer cancel()

		if err := sqlDB.PingContext(checkCtx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not_ready",
				"error":  "database unavailable",
			})
			return
		}

		if err := rdb.Ping(checkCtx).Err(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not_ready",
				"error":  "redis unavailable",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":   "ready",
			"postgres": "connected",
			"redis":    "connected",
		})
	})

	// Setup HTTP server
	server := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown
	serverCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	go func() {
		<-serverCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		log.Println("Shutting down API server...")

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Println("Shutdown error:", err)
		}
	}()

	// Start HTTP server
	log.Printf("API listening on :%s", cfg.AppPort)

	if err := server.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatal("HTTP server error: ", err)
	}
}

// writeJSON writes a JSON HTTP response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Println("Gagal menulis response JSON:", err)
	}
}
