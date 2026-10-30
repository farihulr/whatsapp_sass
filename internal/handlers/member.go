package handlers

import (
	"context"
	"net/http"
	"time"

	"whatsapp-saas/internal/middleware"
	"whatsapp-saas/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MemberHandler struct {
	DB *gorm.DB
}

func NewMemberHandler(db *gorm.DB) *MemberHandler {
	return &MemberHandler{
		DB: db,
	}
}

type MemberResponse struct {
	UserID   uuid.UUID `json:"user_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// List menampilkan semua anggota organisasi.
func (h *MemberHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sub, ok := middleware.CognitoSubject(ctx)
	if !ok || sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "unauthorized",
		})
		return
	}

	organizationID, err := uuid.Parse(r.PathValue("organizationID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid organization ID",
		})
		return
	}

	var user models.User
	if err := h.DB.WithContext(ctx).
		Where("cognito_sub = ?", sub).
		First(&user).Error; err != nil {

		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "user profile not found",
			})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to find user",
		})
		return
	}

	// Verifikasi bahwa caller adalah anggota organisasi.
	var callerMembership models.Membership
	if err := h.DB.WithContext(ctx).
		Where("user_id = ? AND organization_id = ?", user.ID, organizationID).
		First(&callerMembership).Error; err != nil {

		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "you are not a member of this organization",
			})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to verify membership",
		})
		return
	}

	var memberships []models.Membership

	if err := h.DB.WithContext(ctx).
		Preload("User").
		Where("organization_id = ?", organizationID).
		Order("created_at ASC").
		Find(&memberships).Error; err != nil {

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve organization members",
		})
		return
	}

	members := make([]MemberResponse, 0, len(memberships))

	for _, membership := range memberships {
		members = append(members, MemberResponse{
			UserID:   membership.User.ID,
			Email:    membership.User.Email,
			Name:     membership.User.Name,
			Role:     string(membership.Role),
			JoinedAt: membership.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"members": members,
		"total":   len(members),
	})
}

// Get menampilkan detail satu anggota organisasi.
func (h *MemberHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sub, ok := middleware.CognitoSubject(ctx)
	if !ok || sub == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "unauthorized",
		})
		return
	}

	organizationID, err := uuid.Parse(r.PathValue("organizationID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid organization ID",
		})
		return
	}

	targetUserID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid user ID",
		})
		return
	}

	// Cari user yang sedang login.
	var caller models.User
	if err := h.DB.WithContext(ctx).
		Where("cognito_sub = ?", sub).
		First(&caller).Error; err != nil {

		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "user profile not found",
			})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to find user",
		})
		return
	}

	// Pastikan caller adalah anggota organisasi.
	var callerMembership models.Membership
	if err := h.DB.WithContext(ctx).
		Where(
			"user_id = ? AND organization_id = ?",
			caller.ID,
			organizationID,
		).
		First(&callerMembership).Error; err != nil {

		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "you are not a member of this organization",
			})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to verify membership",
		})
		return
	}

	// Cari anggota berdasarkan user ID dan organization ID.
	// Kedua kondisi wajib digunakan agar tidak bisa membaca
	// anggota dari organisasi lain.
	var membership models.Membership

	if err := h.DB.WithContext(ctx).
		Preload("User").
		Where(
			"user_id = ? AND organization_id = ?",
			targetUserID,
			organizationID,
		).
		First(&membership).Error; err != nil {

		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "member not found in this organization",
			})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve member",
		})
		return
	}

	response := MemberResponse{
		UserID:   membership.User.ID,
		Email:    membership.User.Email,
		Name:     membership.User.Name,
		Role:     string(membership.Role),
		JoinedAt: membership.CreatedAt,
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"member": response,
	})
}
