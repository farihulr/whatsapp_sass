package handlers

import (
	"net/http"
	"strings"
)

type WhatsAppWebhookHandler struct {
	VerifyToken string
}

func NewWhatsAppWebhookHandler(verifyToken string) *WhatsAppWebhookHandler {
	return &WhatsAppWebhookHandler{
		VerifyToken: strings.TrimSpace(verifyToken),
	}
}

// Verify menangani verifikasi webhook dari Meta.
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
