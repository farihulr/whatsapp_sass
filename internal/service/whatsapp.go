package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"whatsapp-saas/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrWhatsAppUserNotFound  = errors.New("user profile not found")
	ErrWhatsAppMembership    = errors.New("user is not a member of organization")
	ErrWhatsAppForbidden     = errors.New("insufficient permission")
	ErrWhatsAppNotFound      = errors.New("whatsapp connection not found")
	ErrWhatsAppPhoneRequired = errors.New("phone number ID is required")
)

type WhatsAppService struct {
	db *gorm.DB
}

func NewWhatsAppService(db *gorm.DB) *WhatsAppService {
	return &WhatsAppService{db: db}
}

type CreateWhatsAppInput struct {
	WABAID             *string
	PhoneNumberID      string
	DisplayPhoneNumber *string
	VerifiedName       *string
}

// Mendapatkan user lokal berdasarkan Cognito Sub.
func (s *WhatsAppService) findUser(
	ctx context.Context,
	sub string,
) (*models.User, error) {
	var user models.User

	err := s.db.WithContext(ctx).
		Where("cognito_sub = ?", sub).
		First(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWhatsAppUserNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	return &user, nil
}

// Memastikan user memiliki akses ke organisasi.
func (s *WhatsAppService) authorize(
	ctx context.Context,
	userID uuid.UUID,
	organizationID uuid.UUID,
	requireManage bool,
) error {
	var membership models.Membership

	err := s.db.WithContext(ctx).
		Where(
			"user_id = ? AND organization_id = ?",
			userID,
			organizationID,
		).
		First(&membership).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrWhatsAppMembership
	}

	if err != nil {
		return fmt.Errorf("failed to check membership: %w", err)
	}

	if requireManage &&
		membership.Role != models.RoleOwner &&
		membership.Role != models.RoleAdmin {
		return ErrWhatsAppForbidden
	}

	return nil
}

// CREATE CONNECTION
func (s *WhatsAppService) CreateConnection(
	ctx context.Context,
	sub string,
	organizationID uuid.UUID,
	input CreateWhatsAppInput,
) (*models.WhatsAppConnection, error) {

	phoneNumberID := strings.TrimSpace(input.PhoneNumberID)

	if phoneNumberID == "" {
		return nil, ErrWhatsAppPhoneRequired
	}

	user, err := s.findUser(ctx, sub)
	if err != nil {
		return nil, err
	}

	if err := s.authorize(ctx, user.ID, organizationID, true); err != nil {
		return nil, err
	}

	connection := models.WhatsAppConnection{
		OrganizationID:     organizationID,
		WABAID:             input.WABAID,
		PhoneNumberID:      &phoneNumberID,
		DisplayPhoneNumber: input.DisplayPhoneNumber,
		VerifiedName:       input.VerifiedName,
		Status:             models.WhatsAppPending,
	}

	if err := s.db.WithContext(ctx).Create(&connection).Error; err != nil {
		return nil, fmt.Errorf("failed to create WhatsApp connection: %w", err)
	}

	return &connection, nil
}

// GET CONNECTIONS
func (s *WhatsAppService) GetConnections(
	ctx context.Context,
	sub string,
	organizationID uuid.UUID,
) ([]models.WhatsAppConnection, error) {

	user, err := s.findUser(ctx, sub)
	if err != nil {
		return nil, err
	}

	if err := s.authorize(ctx, user.ID, organizationID, false); err != nil {
		return nil, err
	}

	var connections []models.WhatsAppConnection

	err = s.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("created_at DESC").
		Find(&connections).Error

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve WhatsApp connections: %w", err)
	}

	return connections, nil
}

// DELETE CONNECTION
func (s *WhatsAppService) DeleteConnection(
	ctx context.Context,
	sub string,
	organizationID uuid.UUID,
	connectionID uuid.UUID,
) error {

	user, err := s.findUser(ctx, sub)
	if err != nil {
		return err
	}

	if err := s.authorize(ctx, user.ID, organizationID, true); err != nil {
		return err
	}

	result := s.db.WithContext(ctx).
		Where(
			"id = ? AND organization_id = ?",
			connectionID,
			organizationID,
		).
		Delete(&models.WhatsAppConnection{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete WhatsApp connection: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrWhatsAppNotFound
	}

	return nil
}
