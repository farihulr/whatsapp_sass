package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"whatsapp-saas/internal/middleware"
	"whatsapp-saas/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	errInvitationNotFound = errors.New("invitation not found")
	errInvitationExpired  = errors.New("invitation expired")
	errInvitationUsed     = errors.New("invitation is no longer pending")
	errInvitationEmail    = errors.New("invitation email does not match user email")
	errAlreadyMember      = errors.New("user is already a member")
)

// Accept menerima undangan organisasi menggunakan token undangan.
func (h *InvitationHandler) Accept(w http.ResponseWriter, r *http.Request) {
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
		Token string `json:"token"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON request",
		})
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	if len(req.Token) != 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invitation token tidak valid",
		})
		return
	}

	// Token mentah tidak disimpan di database.
	// Database hanya menyimpan SHA-256 hash dari token.
	tokenBytes, err := hex.DecodeString(req.Token)
	if err != nil || len(tokenBytes) != 32 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invitation token tidak valid",
		})
		return
	}

	digest := sha256.Sum256([]byte(req.Token))
	tokenHash := hex.EncodeToString(digest[:])

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Identitas user diambil dari Cognito, bukan dari request body.
	var user models.User
	if err := h.DB.WithContext(ctx).
		Where("cognito_sub = ?", sub).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "user profile not found; call GET /api/me first",
			})
			return
		}

		log.Println("Gagal mengambil user:", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to retrieve user",
		})
		return
	}

	var accepted models.Invitation

	err = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invitation models.Invitation

		if err := tx.
			Where("token_hash = ?", tokenHash).
			First(&invitation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errInvitationNotFound
			}
			return err
		}

		if invitation.Status != models.InvitationPending {
			return errInvitationUsed
		}

		now := time.Now().UTC()
		if !invitation.ExpiresAt.After(now) {
			if err := tx.Model(&invitation).
				Update("status", models.InvitationExpired).Error; err != nil {
				return err
			}
			return errInvitationExpired
		}

		if !strings.EqualFold(strings.TrimSpace(user.Email), invitation.Email) {
			return errInvitationEmail
		}

		// Cegah user bergabung dua kali ke organisasi yang sama.
		var existing models.Membership
		checkErr := tx.
			Where(
				"user_id = ? AND organization_id = ?",
				user.ID,
				invitation.OrganizationID,
			).
			First(&existing).Error

		if checkErr == nil {
			return errAlreadyMember
		}
		if !errors.Is(checkErr, gorm.ErrRecordNotFound) {
			return checkErr
		}

		// Update bersyarat untuk mencegah token diterima dua kali.
		result := tx.Model(&models.Invitation{}).
			Where(
				"id = ? AND status = ? AND expires_at > ?",
				invitation.ID,
				models.InvitationPending,
				now,
			).
			Updates(map[string]any{
				"status":      models.InvitationAccepted,
				"accepted_at": now,
			})

		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errInvitationUsed
		}

		membership := models.Membership{
			ID:             uuid.New(),
			UserID:         user.ID,
			OrganizationID: invitation.OrganizationID,
			Role:           invitation.Role,
		}

		if err := tx.Create(&membership).Error; err != nil {
			return err
		}

		invitation.Status = models.InvitationAccepted
		invitation.AcceptedAt = &now
		accepted = invitation

		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, errInvitationNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "invitation not found",
			})
		case errors.Is(err, errInvitationExpired):
			writeJSON(w, http.StatusGone, map[string]string{
				"error": "invitation has expired",
			})
		case errors.Is(err, errInvitationUsed):
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "invitation is no longer pending",
			})
		case errors.Is(err, errInvitationEmail):
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "email akun yang login tidak cocok dengan email undangan",
			})
		case errors.Is(err, errAlreadyMember):
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "user is already a member of this organization",
			})
		default:
			log.Println("Gagal menerima invitation:", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to accept invitation",
			})
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":         "invitation accepted successfully",
		"organization_id": accepted.OrganizationID,
		"email":           accepted.Email,
		"role":            accepted.Role,
		"status":          accepted.Status,
		"accepted_at":     accepted.AcceptedAt,
	})
}
