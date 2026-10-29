package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"whatsapp-saas/internal/middleware"
	"whatsapp-saas/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const invitationLifetime = 48 * time.Hour

type InvitationHandler struct {
	DB *gorm.DB
}

func NewInvitationHandler(db *gorm.DB) *InvitationHandler {
	return &InvitationHandler{DB: db}
}

// Create membuat undangan baru untuk admin atau agent.
func (h *InvitationHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	orgID, err := uuid.Parse(r.PathValue("organizationID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "organization ID tidak valid",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		Email string                `json:"email"`
		Role  models.MembershipRole `json:"role"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON request",
		})
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	address, err := mail.ParseAddress(req.Email)
	if err != nil || address.Address != req.Email {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "email tidak valid",
		})
		return
	}

	if req.Role != models.RoleAdmin && req.Role != models.RoleAgent {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "role undangan hanya boleh admin atau agent",
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
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "user profile not found; call GET /api/me first",
			})
			return
		}

		log.Println("Gagal mencari user pengundang:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve user",
		})
		return
	}

	// Pastikan pengundang merupakan owner atau admin organisasi ini.
	var membership models.Membership
	err = h.DB.WithContext(ctx).
		Where(
			"user_id = ? AND organization_id = ?",
			user.ID,
			orgID,
		).
		First(&membership).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "you are not a member of this organization",
		})
		return
	}
	if err != nil {
		log.Println("Gagal memeriksa membership:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to verify organization membership",
		})
		return
	}

	if membership.Role != models.RoleOwner &&
		membership.Role != models.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "only owner or admin can invite members",
		})
		return
	}

	// Pastikan organisasi masih tersedia.
	var org models.Organization
	if err := h.DB.WithContext(ctx).
		First(&org, "id = ?", orgID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "organization not found",
			})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve organization",
		})
		return
	}

	if org.Status != "active" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "organization is not active",
		})
		return
	}

	// Admin tidak boleh mengundang admin baru.
	if membership.Role == models.RoleAdmin &&
		req.Role != models.RoleAgent {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "admin can only invite agents",
		})
		return
	}

	// Jangan undang user yang sudah menjadi anggota organisasi.
	var existingUser models.User
	err = h.DB.WithContext(ctx).
		Where("LOWER(email) = ?", req.Email).
		First(&existingUser).Error

	if err == nil {
		var existingMembership models.Membership
		memberErr := h.DB.WithContext(ctx).
			Where(
				"user_id = ? AND organization_id = ?",
				existingUser.ID,
				orgID,
			).
			First(&existingMembership).Error

		if memberErr == nil {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "user is already a member of this organization",
			})
			return
		}
		if !errors.Is(memberErr, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to check existing membership",
			})
			return
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to check existing user",
		})
		return
	}

	// Buat token acak. Database hanya menyimpan hash-nya.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Println("Gagal membuat token undangan:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to generate invitation token",
		})
		return
	}

	rawToken := hex.EncodeToString(tokenBytes)
	tokenDigest := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(tokenDigest[:])

	now := time.Now().UTC()
	invitation := models.Invitation{
		OrganizationID: orgID,
		InvitedBy:      user.ID,
		Email:          req.Email,
		Role:           req.Role,
		TokenHash:      tokenHash,
		Status:         models.InvitationPending,
		ExpiresAt:      now.Add(invitationLifetime),
	}

	// Izinkan undangan baru jika undangan lama sudah kedaluwarsa.
	err = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous models.Invitation
		findErr := tx.
			Where(
				"organization_id = ? AND email = ? AND status = ?",
				orgID,
				req.Email,
				models.InvitationPending,
			).
			First(&previous).Error

		if findErr == nil {
			if previous.ExpiresAt.After(now) {
				return gorm.ErrDuplicatedKey
			}

			if err := tx.Model(&previous).Update(
				"status",
				models.InvitationExpired,
			).Error; err != nil {
				return err
			}
		} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}

		return tx.Create(&invitation).Error
	})

	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "an active invitation already exists for this email",
			})
			return
		}

		log.Println("Gagal membuat invitation:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to create invitation",
		})
		return
	}

	// PENTING: token hanya dikembalikan untuk pengujian lokal.
	// Sebelum production, kirim token melalui email dan hapus
	// field token dari response API.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":              invitation.ID,
		"organization_id": invitation.OrganizationID,
		"email":           invitation.Email,
		"role":            invitation.Role,
		"status":          invitation.Status,
		"expires_at":      invitation.ExpiresAt,
		"token":           rawToken,
	})
}

// List menampilkan undangan dalam organisasi.
func (h *InvitationHandler) List(w http.ResponseWriter, r *http.Request) {
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

	orgID, err := uuid.Parse(r.PathValue("organizationID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "organization ID tidak valid",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := h.DB.WithContext(ctx).
		Where("cognito_sub = ?", sub).
		First(&user).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "user profile not found",
		})
		return
	}

	var membership models.Membership
	if err := h.DB.WithContext(ctx).
		Where(
			"user_id = ? AND organization_id = ?",
			user.ID,
			orgID,
		).
		First(&membership).Error; err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "you are not a member of this organization",
		})
		return
	}

	if membership.Role != models.RoleOwner &&
		membership.Role != models.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "only owner or admin can view invitations",
		})
		return
	}

	var invitations []models.Invitation
	if err := h.DB.WithContext(ctx).
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Find(&invitations).Error; err != nil {
		log.Println("Gagal mengambil invitations:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve invitations",
		})
		return
	}

	// TokenHash memiliki json:"-" sehingga tidak ikut dikirim.
	if invitations == nil {
		invitations = []models.Invitation{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"invitations": invitations,
	})
}
