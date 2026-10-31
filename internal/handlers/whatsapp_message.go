package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"whatsapp-saas/internal/middleware"
	"whatsapp-saas/internal/models"
	"whatsapp-saas/internal/service"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WhatsAppMessageHandler struct {
	DB     *gorm.DB
	Client *service.WhatsAppClient
}

func NewWhatsAppMessageHandler(
	db *gorm.DB,
	client *service.WhatsAppClient,
) *WhatsAppMessageHandler {
	return &WhatsAppMessageHandler{
		DB:     db,
		Client: client,
	}
}

type sendWhatsAppMessageRequest struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

type sendWhatsAppMessageResponse struct {
	Status    string `json:"status"`
	MessageID string `json:"message_id"`
}

func (h *WhatsAppMessageHandler) Send(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWhatsAppJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "method not allowed",
		})
		return
	}

	if h.DB == nil || h.Client == nil {
		writeWhatsAppJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "WhatsApp service belum tersedia",
		})
		return
	}

	// Ambil Cognito Subject dari middleware.
	sub, ok := middleware.CognitoSubject(r.Context())
	if !ok || sub == "" {
		writeWhatsAppJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "unauthorized",
		})
		return
	}

	// Ambil Organization ID dari URL.
	organizationID, err := uuid.Parse(r.PathValue("organizationID"))
	if err != nil {
		writeWhatsAppJSON(w, http.StatusBadRequest, map[string]string{
			"error": "organization ID tidak valid",
		})
		return
	}

	// Cari user lokal berdasarkan Cognito Subject.
	var user models.User
	if err := h.DB.WithContext(r.Context()).
		Where("cognito_sub = ?", sub).
		First(&user).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeWhatsAppJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "user profile tidak ditemukan",
			})
			return
		}

		writeWhatsAppJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "gagal memeriksa user",
		})
		return
	}

	// Pastikan user merupakan anggota organisasi.
	var membership models.Membership
	if err := h.DB.WithContext(r.Context()).
		Where(
			"user_id = ? AND organization_id = ?",
			user.ID,
			organizationID,
		).
		First(&membership).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeWhatsAppJSON(w, http.StatusForbidden, map[string]string{
				"error": "kamu bukan anggota organisasi ini",
			})
			return
		}

		writeWhatsAppJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "gagal memeriksa membership",
		})
		return
	}

	// Decode request body.
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)

	var input sendWhatsAppMessageRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		writeWhatsAppJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request body tidak valid",
		})
		return
	}

	// Tolak JSON tambahan setelah object pertama.
	var extra interface{}
	if err := decoder.Decode(&extra); err == nil {
		writeWhatsAppJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request body hanya boleh berisi satu JSON object",
		})
		return
	}

	input.To = normalizeWhatsAppNumber(input.To)
	input.Body = strings.TrimSpace(input.Body)

	if input.To == "" {
		writeWhatsAppJSON(w, http.StatusBadRequest, map[string]string{
			"error": "nomor tujuan tidak valid",
		})
		return
	}

	if input.Body == "" {
		writeWhatsAppJSON(w, http.StatusBadRequest, map[string]string{
			"error": "isi pesan tidak boleh kosong",
		})
		return
	}

	if len(input.Body) > 4096 {
		writeWhatsAppJSON(w, http.StatusBadRequest, map[string]string{
			"error": "isi pesan maksimal 4096 karakter",
		})
		return
	}

	// Kirim pesan ke Meta WhatsApp Cloud API.
	result, err := h.Client.SendTextMessage(
		r.Context(),
		service.SendTextMessageRequest{
			To:   input.To,
			Body: input.Body,
		},
	)
	if err != nil {
		writeWhatsAppJSON(w, http.StatusBadGateway, map[string]string{
			"error": "gagal mengirim pesan melalui WhatsApp API",
		})
		return
	}

	writeWhatsAppJSON(w, http.StatusOK, sendWhatsAppMessageResponse{
		Status:    "accepted",
		MessageID: result.Messages[0].ID,
	})
}

// Menghapus karakter pemisah nomor telepon.
// Contoh: +62 823-3389-9903 menjadi 6282333899903.
func normalizeWhatsAppNumber(number string) string {
	number = strings.TrimSpace(number)
	number = strings.NewReplacer(
		"+", "",
		"-", "",
		" ", "",
		"(", "",
		")", "",
	).Replace(number)

	for _, char := range number {
		if char < '0' || char > '9' {
			return ""
		}
	}

	return number
}

func writeWhatsAppJSON(
	w http.ResponseWriter,
	status int,
	data interface{},
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
