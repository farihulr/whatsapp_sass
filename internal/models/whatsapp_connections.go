package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WhatsAppConnectionStatus string

const (
	WhatsAppPending      WhatsAppConnectionStatus = "pending"
	WhatsAppActive       WhatsAppConnectionStatus = "active"
	WhatsAppDisconnected WhatsAppConnectionStatus = "disconnected"
	WhatsAppError        WhatsAppConnectionStatus = "error"
)

type WhatsAppConnection struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index" json:"organization_id"`

	// Meta WhatsApp Business identifiers
	WABAID             *string `gorm:"type:varchar(100)" json:"waba_id,omitempty"`
	PhoneNumberID      *string `gorm:"type:varchar(100);uniqueIndex" json:"phone_number_id,omitempty"`
	DisplayPhoneNumber *string `gorm:"type:varchar(30)" json:"display_phone_number,omitempty"`
	VerifiedName       *string `gorm:"type:varchar(255)" json:"verified_name,omitempty"`

	// Store encrypted token only, never plaintext
	AccessTokenEncrypted []byte `gorm:"type:bytea" json:"-"`

	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`

	Status WhatsAppConnectionStatus `gorm:"type:varchar(20);not null;default:pending;check:chk_whatsapp_connection_status,status IN ('pending','active','disconnected','error')" json:"status"`

	ConnectedAt  *time.Time `json:"connected_at,omitempty"`
	SubscribedAt *time.Time `json:"subscribed_at,omitempty"`
	LastError    *string    `gorm:"type:text" json:"last_error,omitempty"`

	Organization Organization `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (w *WhatsAppConnection) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}

	if w.Status == "" {
		w.Status = WhatsAppPending
	}

	return nil
}
