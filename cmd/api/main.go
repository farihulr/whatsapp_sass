package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"whatsapp-saas/internal/config"
	"whatsapp-saas/internal/database"
	"whatsapp-saas/internal/middleware"
	"whatsapp-saas/internal/router"
	"whatsapp-saas/internal/service"

	redisClient "whatsapp-saas/internal/redis"

	"github.com/aws/aws-sdk-go-v2/aws"
	cognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
)

func main() {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Gagal memuat konfigurasi: ", err)
	}

	ctx := context.Background()

	// Connect PostgreSQL.
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

	// Database migrations.
	if err := database.AutoMigrate(db); err != nil {
		log.Fatal("Gagal melakukan database migration: ", err)
	}

	log.Println("Database migration completed")

	// Connect Redis.
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

	// Initialize Cognito API client.
	cognitoClient := cognito.NewFromConfig(aws.Config{
		Region: cfg.CognitoRegion,
	})

	// Initialize Cognito token-validation middleware.
	authMiddleware, err := middleware.NewCognitoMiddleware(
		ctx,
		cfg.CognitoRegion,
		cfg.CognitoUserPoolID,
		cfg.CognitoAppClientID,
	)
	if err != nil {
		log.Fatal("Gagal menginisialisasi Cognito middleware: ", err)
	}

	// Initialize Cognito profile service.
	cognitoProfileService := service.NewCognitoProfileService(
		cognitoClient,
	)

	// Initialize Cognito registration and email-confirmation service.
	cognitoAuthService := service.NewCognitoAuthService(
		cognitoClient,
		cfg.CognitoAppClientID,
		cfg.CognitoAppClientSecret,
	)

	log.Println("Cognito authentication and registration initialized")

	// Build HTTP router.
	handler := router.New(router.Dependencies{
		DB:                    db,
		Redis:                 rdb,
		AuthMiddleware:        authMiddleware,
		CognitoProfileService: cognitoProfileService,
		CognitoAuthService:    cognitoAuthService,
	})

	server := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown.
	serverCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	go func() {
		<-serverCtx.Done()

		log.Println("Shutting down API server...")

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Println("Shutdown error:", err)
		}
	}()

	log.Printf("API listening on :%s", cfg.AppPort)

	if err := server.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatal("HTTP server error: ", err)
	}
}
