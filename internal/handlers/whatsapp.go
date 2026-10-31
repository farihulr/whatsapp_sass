package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"whatsapp-saas/internal/middleware"
	"whatsapp-saas/internal/service"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WhatsAppHandler struct {
	Service *service.WhatsAppService
}

func NewWhatsAppHandler(s *service.WhatsAppService) *WhatsAppHandler {
	return &WhatsAppHandler{
		Service: s,
	}
}

type CreateWhatsAppRequest struct {
	WABAID             *string `json:"waba_id"`
	PhoneNumberID      string  `json:"phone_number_id"`
	DisplayPhoneNumber *string `json:"display_phone_number"`
	VerifiedName       *string `json:"verified_name"`
}

func (h *WhatsAppHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sub, ok := middleware.CognitoSubject(ctx)
	if !ok {
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

	var req CreateWhatsAppRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	connection, err := h.Service.CreateConnection(
		ctx,
		sub,
		organizationID,
		service.CreateWhatsAppInput{
			WABAID:             req.WABAID,
			PhoneNumberID:      req.PhoneNumberID,
			DisplayPhoneNumber: req.DisplayPhoneNumber,
			VerifiedName:       req.VerifiedName,
		},
	)

	if err != nil {
		h.handleError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message":    "WhatsApp connection created successfully",
		"connection": connection,
	})
}

func (h *WhatsAppHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sub, ok := middleware.CognitoSubject(ctx)
	if !ok {
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

	connections, err := h.Service.GetConnections(
		ctx,
		sub,
		organizationID,
	)

	if err != nil {
		h.handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connections": connections,
		"total":       len(connections),
	})
}

func (h *WhatsAppHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sub, ok := middleware.CognitoSubject(ctx)
	if !ok {
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

	connectionID, err := uuid.Parse(r.PathValue("connectionID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid connection ID",
		})
		return
	}

	err = h.Service.DeleteConnection(
		ctx,
		sub,
		organizationID,
		connectionID,
	)

	if err != nil {
		h.handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "WhatsApp connection deleted successfully",
	})
}

func (h *WhatsAppHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrWhatsAppUserNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "user profile not found",
		})

	case errors.Is(err, service.ErrWhatsAppMembership):
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "you are not a member of this organization",
		})

	case errors.Is(err, service.ErrWhatsAppForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "you do not have permission to manage WhatsApp",
		})

	case errors.Is(err, service.ErrWhatsAppNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "WhatsApp connection not found",
		})

	case errors.Is(err, service.ErrWhatsAppPhoneRequired):
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "phone_number_id is required",
		})

	default:
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "resource not found",
			})
			return
		}

		log.Printf("WhatsApp handler error: %v", err)

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "internal server error",
		})
	}
}
