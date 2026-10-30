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
	CognitoAuthService    *service.CognitoAuthService
}

func New(deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	// =====================================
	// HEALTH CHECK
	// =====================================

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	// =====================================
	// READINESS CHECK
	// =====================================

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

	// =====================================
	// PUBLIC AUTHENTICATION ROUTES
	// =====================================

	authHandler := handlers.NewAuthHandler(deps.CognitoAuthService)

	mux.HandleFunc("POST /api/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/auth/confirm", authHandler.Confirm)
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)

	// =====================================
	// INITIALIZE HANDLERS
	// =====================================

	meHandler := handlers.NewMeHandler(
		deps.DB,
		deps.CognitoProfileService,
	)

	memberHandler := handlers.NewMemberHandler(deps.DB)

	organizationHandler := handlers.NewOrganizationHandler(deps.DB)

	invitationHandler := handlers.NewInvitationHandler(deps.DB)

	// =====================================
	// AUTH MIDDLEWARE CHECK
	// =====================================

	if deps.AuthMiddleware == nil {
		unavailable := func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "authentication service unavailable",
			})
		}

		// User routes
		mux.HandleFunc("GET /api/me", unavailable)

		// Organization routes
		mux.HandleFunc("POST /api/organizations", unavailable)
		mux.HandleFunc("GET /api/organizations", unavailable)

		// Invitation routes
		mux.HandleFunc(
			"POST /api/organizations/{organizationID}/invitations",
			unavailable,
		)

		mux.HandleFunc(
			"GET /api/organizations/{organizationID}/invitations",
			unavailable,
		)

		mux.HandleFunc(
			"POST /api/invitations/accept",
			unavailable,
		)

		// Member routes
		mux.HandleFunc(
			"GET /api/organizations/{organizationID}/members",
			unavailable,
		)

		mux.HandleFunc(
			"GET /api/organizations/{organizationID}/members/{userID}",
			unavailable,
		)

		mux.HandleFunc(
			"PATCH /api/organizations/{organizationID}/members/{userID}",
			unavailable,
		)

		return mux
	}

	// =====================================
	// PROTECTED USER ROUTES
	// =====================================

	mux.Handle(
		"GET /api/me",
		deps.AuthMiddleware.RequireAuth(meHandler),
	)

	// =====================================
	// ORGANIZATION ROUTES
	// =====================================

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

	// =====================================
	// INVITATION ROUTES
	// =====================================

	mux.Handle(
		"POST /api/organizations/{organizationID}/invitations",
		deps.AuthMiddleware.RequireAuth(
			http.HandlerFunc(invitationHandler.Create),
		),
	)

	mux.Handle(
		"GET /api/organizations/{organizationID}/invitations",
		deps.AuthMiddleware.RequireAuth(
			http.HandlerFunc(invitationHandler.List),
		),
	)

	mux.Handle(
		"POST /api/invitations/accept",
		deps.AuthMiddleware.RequireAuth(
			http.HandlerFunc(invitationHandler.Accept),
		),
	)

	// =====================================
	// MEMBER ROUTES
	// =====================================

	// Melihat daftar anggota organisasi.
	mux.Handle(
		"GET /api/organizations/{organizationID}/members",
		deps.AuthMiddleware.RequireAuth(
			http.HandlerFunc(memberHandler.List),
		),
	)

	// Melihat detail anggota organisasi.
	mux.Handle(
		"GET /api/organizations/{organizationID}/members/{userID}",
		deps.AuthMiddleware.RequireAuth(
			http.HandlerFunc(memberHandler.Get),
		),
	)

	// Mengubah role anggota organisasi.
	mux.Handle(
		"PATCH /api/organizations/{organizationID}/members/{userID}",
		deps.AuthMiddleware.RequireAuth(
			http.HandlerFunc(memberHandler.UpdateRole),
		),
	)

	return mux
}

// =====================================
// JSON RESPONSE HELPER
// =====================================

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Println("Gagal menulis response JSON:", err)
	}
}
