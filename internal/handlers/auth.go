package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"whatsapp-saas/internal/service"
)

type AuthHandler struct {
	CognitoAuthService *service.CognitoAuthService
}

func NewAuthHandler(authService *service.CognitoAuthService) *AuthHandler {
	return &AuthHandler{
		CognitoAuthService: authService,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if h.CognitoAuthService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "authentication service unavailable",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request JSON tidak valid",
		})
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	address, err := mail.ParseAddress(req.Email)
	if err != nil || address.Address != req.Email {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "email tidak valid",
		})
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "nama wajib diisi dan maksimal 100 karakter",
		})
		return
	}

	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "password minimal 8 karakter; kebijakan Cognito juga berlaku",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.CognitoAuthService.Register(
		ctx,
		req.Email,
		req.Name,
		req.Password,
	); err != nil {
		log.Printf("Registrasi Cognito gagal: %v", err)

		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "registrasi gagal; periksa email, kebijakan password, dan konfigurasi Cognito",
		})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"message": "registrasi berhasil; periksa email untuk kode verifikasi",
		"email":   req.Email,
	})
}

func (h *AuthHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	if h.CognitoAuthService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "authentication service unavailable",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request JSON tidak valid",
		})
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)

	address, err := mail.ParseAddress(req.Email)
	if err != nil || address.Address != req.Email || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "email atau kode verifikasi tidak valid",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.CognitoAuthService.ConfirmRegistration(
		ctx,
		req.Email,
		req.Code,
	); err != nil {
		log.Printf("Verifikasi Cognito gagal: %v", err)

		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "verifikasi gagal; periksa kode dan email",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "email berhasil diverifikasi; silakan login",
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.CognitoAuthService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "authentication service unavailable",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request JSON tidak valid",
		})
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "email dan password wajib diisi",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := h.CognitoAuthService.Login(ctx, req.Email, req.Password)
	if err != nil {
		log.Printf("Login Cognito gagal: %v", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "login gagal; periksa email dan password",
		})
		return
	}

	auth := result.AuthenticationResult
	response := map[string]any{
		"token_type":   "Bearer",
		"expires_in":   auth.ExpiresIn,
		"access_token": auth.AccessToken,
		"id_token":     auth.IdToken,
	}

	if auth.RefreshToken != nil {
		response["refresh_token"] = *auth.RefreshToken
	}

	writeJSON(w, http.StatusOK, response)
}
