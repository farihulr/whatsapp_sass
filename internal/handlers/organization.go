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

	"gorm.io/gorm"
)

type OrganizationHandler struct {
	DB *gorm.DB
}

func NewOrganizationHandler(db *gorm.DB) *OrganizationHandler {
	return &OrganizationHandler{DB: db}
}

func (h *OrganizationHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "database unavailable",
		})
		return
	}

	sub, ok := middleware.CognitoSubject(r.Context())
	if !ok || sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "user identity not found",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON request",
		})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))

	if req.Name == "" || req.Slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "name dan slug wajib diisi",
		})
		return
	}

	if len(req.Name) > 150 || len(req.Slug) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "name maksimal 150 karakter dan slug maksimal 100 karakter",
		})
		return
	}

	// Slug hanya boleh menggunakan huruf kecil, angka, dan tanda hubung.
	for _, char := range req.Slug {
		if !((char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') ||
			char == '-') {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "slug hanya boleh berisi huruf kecil, angka, dan tanda hubung",
			})
			return
		}
	}

	if req.Slug[0] == '-' || req.Slug[len(req.Slug)-1] == '-' {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "slug tidak boleh diawali atau diakhiri tanda hubung",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var user models.User
	err := h.DB.WithContext(ctx).
		Where("cognito_sub = ?", sub).
		First(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "user profile not found; call GET /api/me first",
		})
		return
	}
	if err != nil {
		log.Println("Gagal mencari user untuk organization:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve user",
		})
		return
	}

	org := models.Organization{
		Name:   req.Name,
		Slug:   req.Slug,
		Status: "active",
	}

	err = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&org).Error; err != nil {
			return err
		}

		membership := models.Membership{
			UserID:         user.ID,
			OrganizationID: org.ID,
			Role:           models.RoleOwner,
		}

		return tx.Create(&membership).Error
	})

	if err != nil {
		log.Println("Gagal membuat organization dan membership:", err)
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "failed to create organization; check whether the slug is already used",
		})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     org.ID,
		"name":   org.Name,
		"slug":   org.Slug,
		"status": org.Status,
		"role":   string(models.RoleOwner),
	})
}

func (h *OrganizationHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "database unavailable",
		})
		return
	}

	sub, ok := middleware.CognitoSubject(r.Context())
	if !ok || sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "user identity not found",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := h.DB.WithContext(ctx).
		Where("cognito_sub = ?", sub).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{
				"organizations": []any{},
			})
			return
		}

		log.Println("Gagal mencari user:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve user",
		})
		return
	}

	var memberships []models.Membership
	if err := h.DB.WithContext(ctx).
		Preload("Organization").
		Where("user_id = ?", user.ID).
		Find(&memberships).Error; err != nil {
		log.Println("Gagal mengambil memberships:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve organizations",
		})
		return
	}

	organizations := make([]map[string]any, 0, len(memberships))
	for _, membership := range memberships {
		if membership.Organization.ID == (user.ID) {
			// Tidak perlu mengembalikan organisasi yang tidak valid.
			continue
		}

		organizations = append(organizations, map[string]any{
			"id":     membership.Organization.ID,
			"name":   membership.Organization.Name,
			"slug":   membership.Organization.Slug,
			"status": membership.Organization.Status,
			"role":   membership.Role,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"organizations": organizations,
	})
}