package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"whatsapp-saas/internal/middleware"
	"whatsapp-saas/internal/models"
	"whatsapp-saas/internal/service"

	"gorm.io/gorm"
)

type MeHandler struct {
	DB                    *gorm.DB
	CognitoProfileService *service.CognitoProfileService
}

func NewMeHandler(
	db *gorm.DB,
	profileService *service.CognitoProfileService,
) *MeHandler {
	return &MeHandler{
		DB:                    db,
		CognitoProfileService: profileService,
	}
}

func (h *MeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil || h.CognitoProfileService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "user profile service unavailable",
		})
		return
	}

	// Ambil subject dari access token yang sudah diverifikasi middleware.
	sub, ok := middleware.CognitoSubject(r.Context())
	if !ok || sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "user identity not found",
		})
		return
	}

	// Ambil access token untuk meminta profil lengkap ke Cognito.
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Bearer token diperlukan",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	profile, err := h.CognitoProfileService.GetProfile(ctx, parts[1])
	if err != nil {
		log.Println("Gagal mengambil profil Cognito:", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "failed to retrieve Cognito profile",
		})
		return
	}

	// Pastikan profil Cognito berasal dari user yang sama dengan token.
	if profile.Sub != sub {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Cognito identity mismatch",
		})
		return
	}

	// Hanya izinkan pembuatan profil jika email sudah diverifikasi.
	if !profile.EmailVerified {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "email must be verified before creating a profile",
		})
		return
	}

	var user models.User

	err = h.DB.WithContext(ctx).
		Where("cognito_sub = ?", sub).
		First(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// User belum ada di PostgreSQL, jadi buat profil baru.
		user = models.User{
			CognitoSub: sub,
			Email:      profile.Email,
			Name:       profile.Name,
			Status:     "active",
		}

		if createErr := h.DB.WithContext(ctx).Create(&user).Error; createErr != nil {
			log.Println("Gagal membuat profil user:", createErr)
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "could not create user profile; check email uniqueness and database constraints",
			})
			return
		}
	} else if err != nil {
		log.Println("Gagal mencari profil user:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve user profile",
		})
		return
	} else {
		// User sudah ada; perbarui data dari Cognito.
		updates := map[string]any{
			"email": profile.Email,
			"name":  profile.Name,
		}

		if updateErr := h.DB.WithContext(ctx).
			Model(&user).
			Updates(updates).Error; updateErr != nil {
			log.Println("Gagal memperbarui profil user:", updateErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to update user profile",
			})
			return
		}

		user.Email = profile.Email
		user.Name = profile.Name
	}

	// Kirim profil aplikasi ke client.
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         user.ID,
		"email":      user.Email,
		"name":       user.Name,
		"status":     user.Status,
		"created_at": user.CreatedAt,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Println("Gagal menulis response JSON:", err)
	}
}
