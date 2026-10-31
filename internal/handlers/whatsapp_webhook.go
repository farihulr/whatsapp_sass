package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"whatsapp-saas/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxWebhookBodySize = 2 * 1024 * 1024

type WhatsAppWebhookHandler struct {
	DB          *gorm.DB
	VerifyToken string
	AppSecret   string
}

func NewWhatsAppWebhookHandler(
	db *gorm.DB,
	verifyToken string,
	appSecret string,
) *WhatsAppWebhookHandler {
	return &WhatsAppWebhookHandler{
		DB:          db,
		VerifyToken: strings.TrimSpace(verifyToken),
		AppSecret:   strings.TrimSpace(appSecret),
	}
}

// Struktur payload webhook WhatsApp Cloud API.

type whatsappWebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Value whatsappWebhookValue `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type whatsappWebhookValue struct {
	Metadata struct {
		PhoneNumberID string `json:"phone_number_id"`
	} `json:"metadata"`

	Contacts []struct {
		WaID    string `json:"wa_id"`
		Profile struct {
			Name string `json:"name"`
		} `json:"profile"`
	} `json:"contacts"`

	Messages []whatsappIncomingMessage `json:"messages"`
	Statuses []whatsappMessageStatus   `json:"statuses"`
}

type whatsappIncomingMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      *struct {
		Body string `json:"body"`
	} `json:"text"`
	Image *struct {
		Caption string `json:"caption"`
	} `json:"image"`
	Document *struct {
		Caption string `json:"caption"`
	} `json:"document"`
	Video *struct {
		Caption string `json:"caption"`
	} `json:"video"`
}

type whatsappMessageStatus struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Timestamp   string `json:"timestamp"`
	RecipientID string `json:"recipient_id"`
}

// Verify menangani verifikasi webhook GET dari Meta.
func (h *WhatsAppWebhookHandler) Verify(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.VerifyToken == "" {
		http.Error(
			w,
			"webhook verify token belum dikonfigurasi",
			http.StatusServiceUnavailable,
		)
		return
	}

	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode != "subscribe" {
		http.Error(w, "invalid webhook mode", http.StatusForbidden)
		return
	}

	if token != h.VerifyToken {
		http.Error(w, "invalid verify token", http.StatusForbidden)
		return
	}

	if challenge == "" {
		http.Error(w, "missing challenge", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(challenge))
}

// Receive menangani pesan dan status pesan dari Meta.
func (h *WhatsAppWebhookHandler) Receive(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.DB == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	if h.AppSecret == "" {
		http.Error(w, "webhook app secret belum dikonfigurasi", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodySize)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid webhook body", http.StatusBadRequest)
		return
	}

	// Validasi signature sebelum memproses payload.
	if !h.verifySignature(body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
		return
	}

	var payload whatsappWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid webhook JSON", http.StatusBadRequest)
		return
	}

	// Abaikan object yang bukan webhook WhatsApp Business.
	if payload.Object != "whatsapp_business_account" {
		w.WriteHeader(http.StatusOK)
		return
	}

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			value := change.Value

			phoneNumberID := strings.TrimSpace(
				value.Metadata.PhoneNumberID,
			)

			if phoneNumberID == "" {
				continue
			}

			var connection models.WhatsAppConnection

			err := h.DB.
				Where("phone_number_id = ?", phoneNumberID).
				First(&connection).Error

			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf(
					"WhatsApp webhook: connection tidak ditemukan untuk phone_number_id %s",
					phoneNumberID,
				)
				continue
			}

			if err != nil {
				log.Printf("WhatsApp webhook: gagal mencari connection: %v", err)
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}

			// Simpan setiap pesan masuk.
			for _, message := range value.Messages {
				customerName := findCustomerName(
					value.Contacts,
					message.From,
				)

				if err := h.saveIncomingMessage(
					&connection,
					message,
					customerName,
				); err != nil {
					log.Printf(
						"WhatsApp webhook: gagal menyimpan pesan: %v",
						err,
					)
					http.Error(w, "failed to process message", http.StatusInternalServerError)
					return
				}
			}

			// Perbarui status pesan keluar.
			for _, status := range value.Statuses {
				if err := h.updateMessageStatus(status); err != nil {
					log.Printf(
						"WhatsApp webhook: gagal memperbarui status pesan: %v",
						err,
					)
					http.Error(w, "failed to update message status", http.StatusInternalServerError)
					return
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

// Memvalidasi HMAC SHA-256 dari raw request body.
func (h *WhatsAppWebhookHandler) verifySignature(
	body []byte,
	signature string,
) bool {
	signature = strings.TrimSpace(signature)

	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	providedSignature, err := hex.DecodeString(
		strings.TrimPrefix(signature, "sha256="),
	)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.AppSecret))
	_, _ = mac.Write(body)

	expectedSignature := mac.Sum(nil)

	return hmac.Equal(providedSignature, expectedSignature)
}

// Mencari nama pelanggan berdasarkan nomor WhatsApp.
func findCustomerName(
	contacts []struct {
		WaID    string `json:"wa_id"`
		Profile struct {
			Name string `json:"name"`
		} `json:"profile"`
	},
	phone string,
) *string {
	for _, contact := range contacts {
		if contact.WaID == phone && strings.TrimSpace(contact.Profile.Name) != "" {
			name := strings.TrimSpace(contact.Profile.Name)
			return &name
		}
	}

	return nil
}

// Menyimpan pesan masuk dan conversation pelanggan.
func (h *WhatsAppWebhookHandler) saveIncomingMessage(
	connection *models.WhatsAppConnection,
	incoming whatsappIncomingMessage,
	customerName *string,
) error {
	if strings.TrimSpace(incoming.ID) == "" ||
		strings.TrimSpace(incoming.From) == "" {
		return fmt.Errorf("message ID atau nomor pengirim kosong")
	}

	messageAt := parseWhatsAppTimestamp(incoming.Timestamp)

	messageType := mapWhatsAppMessageType(incoming.Type)
	messageBody := extractMessageBody(incoming)

	preview := "Pesan " + string(messageType)
	if messageBody != nil {
		preview = *messageBody
	}
	if len(preview) > 255 {
		preview = preview[:255]
	}

	rawPayload, err := json.Marshal(incoming)
	if err != nil {
		return fmt.Errorf("gagal encode raw message: %w", err)
	}

	return h.DB.Transaction(func(tx *gorm.DB) error {
		var conversation models.Conversation

		err := tx.
			Where(
				"organization_id = ? AND whatsapp_connection_id = ? AND customer_phone = ?",
				connection.OrganizationID,
				connection.ID,
				incoming.From,
			).
			First(&conversation).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			conversation = models.Conversation{
				OrganizationID:       connection.OrganizationID,
				WhatsAppConnectionID: connection.ID,
				CustomerPhone:        incoming.From,
				CustomerName:         customerName,
				Status:               models.ConversationOpen,
			}

			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "organization_id"},
					{Name: "whatsapp_connection_id"},
					{Name: "customer_phone"},
				},
				DoNothing: true,
			}).Create(&conversation).Error; err != nil {
				return fmt.Errorf("gagal membuat conversation: %w", err)
			}

			// Ambil ulang agar aman jika dua webhook membuat conversation
			// untuk pelanggan yang sama secara bersamaan.
			if err := tx.
				Where(
					"organization_id = ? AND whatsapp_connection_id = ? AND customer_phone = ?",
					connection.OrganizationID,
					connection.ID,
					incoming.From,
				).
				First(&conversation).Error; err != nil {
				return fmt.Errorf("gagal mengambil conversation: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("gagal mencari conversation: %w", err)
		}

		// Simpan pesan hanya jika WhatsApp Message ID belum pernah ada.
		whatsappMessageID := incoming.ID

		message := models.Message{
			ConversationID:    conversation.ID,
			WhatsAppMessageID: &whatsappMessageID,
			Direction:         models.MessageInbound,
			Type:              messageType,
			Status:            models.MessageReceived,
			Body:              messageBody,
			MessageAt:         messageAt,
			RawPayload:        rawPayload,
		}

		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "whatsapp_message_id"},
			},
			DoNothing: true,
		}).Create(&message)

		if result.Error != nil {
			return fmt.Errorf("gagal menyimpan message: %w", result.Error)
		}

		// Jangan menambah unread count jika pesan merupakan duplikat.
		if result.RowsAffected == 0 {
			return nil
		}

		updates := map[string]interface{}{
			"last_message_at":      messageAt,
			"last_message_preview": preview,
			"unread_count":         gorm.Expr("unread_count + ?", 1),
		}

		if customerName != nil {
			updates["customer_name"] = customerName
		}

		if err := tx.
			Model(&models.Conversation{}).
			Where("id = ?", conversation.ID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("gagal memperbarui conversation: %w", err)
		}

		return nil
	})
}

// Memperbarui status pesan berdasarkan notifikasi Meta.
func (h *WhatsAppWebhookHandler) updateMessageStatus(
	status whatsappMessageStatus,
) error {
	if status.ID == "" {
		return nil
	}

	var messageStatus models.MessageStatus

	switch strings.ToLower(status.Status) {
	case "sent":
		messageStatus = models.MessageSent
	case "delivered":
		messageStatus = models.MessageDelivered
	case "read":
		messageStatus = models.MessageRead
	case "failed":
		messageStatus = models.MessageFailed
	default:
		return nil
	}

	return h.DB.
		Model(&models.Message{}).
		Where("whatsapp_message_id = ?", status.ID).
		Update("status", messageStatus).Error
}

func mapWhatsAppMessageType(value string) models.MessageType {
	switch strings.ToLower(value) {
	case "text":
		return models.MessageText
	case "image":
		return models.MessageImage
	case "audio":
		return models.MessageAudio
	case "video":
		return models.MessageVideo
	case "document":
		return models.MessageDocument
	case "location":
		return models.MessageLocation
	case "interactive":
		return models.MessageInteractive
	default:
		return models.MessageUnknown
	}
}

func extractMessageBody(message whatsappIncomingMessage) *string {
	var body string

	switch {
	case message.Text != nil:
		body = message.Text.Body
	case message.Image != nil:
		body = message.Image.Caption
	case message.Document != nil:
		body = message.Document.Caption
	case message.Video != nil:
		body = message.Video.Caption
	default:
		return nil
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}

	return &body
}

func parseWhatsAppTimestamp(value string) time.Time {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Now().UTC()
	}

	return time.Unix(seconds, 0).UTC()
}
