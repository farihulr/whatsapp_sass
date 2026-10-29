package router

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"whatsapp-saas/internal/handlers"
	"whatsapp-saas/internal/middleware"
	"whatsapp-saas/internal/service"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Dependencies struct {
	DB                    *gorm.DB
	Redis                 *redis.Client
	AuthMiddleware        *middleware.CognitoMiddleware
	CognitoProfileService *service.CognitoProfileService
}

func New(deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	// Liveness check: memastikan HTTP server berjalan.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	// Readiness check: memastikan database dan Redis siap.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if deps.DB == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not_ready",
				"error":  "database unavailable",
			})
			return
		}

		sqlDB, err := deps.DB.DB()
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not_ready",
				"error":  "database unavailable",
			})
			return
		}

		if err := sqlDB.PingContext(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not_ready",
				"error":  "database unavailable",
			})
			return
		}

		if deps.Redis == nil || deps.Redis.Ping(ctx).Err() != nil {
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

	// Siapkan handler.
	meHandler := handlers.NewMeHandler(
		deps.DB,
		deps.CognitoProfileService,
	)

	organizationHandler := handlers.NewOrganizationHandler(deps.DB)

	// Semua endpoint privat harus melewati middleware Cognito.
	if deps.AuthMiddleware == nil {
		unavailable := func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "authentication service unavailable",
			})
		}

		mux.HandleFunc("GET /api/me", unavailable)
		mux.HandleFunc("POST /api/organizations", unavailable)

		return mux
	}

	mux.Handle(
		"GET /api/me",
		deps.AuthMiddleware.RequireAuth(meHandler),
	)

	mux.Handle(
		"POST /api/organizations",
		deps.AuthMiddleware.RequireAuth(
			http.HandlerFunc(organizationHandler.Create),
		),
	)

	mux.Handle(
	"GET /api/organizations",
	deps.AuthMiddleware.RequireAuth(
		http.HandlerFunc(organizationHandler.List),
	),
)

	return mux
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Println("Gagal menulis response JSON:", err)
	}
}
