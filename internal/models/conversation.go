package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConversationStatus string

const (
	ConversationOpen   ConversationStatus = "open"
	ConversationClosed ConversationStatus = "closed"
)

type Conversation struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`

	// Organisasi pemilik conversation.
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_conversation_customer,priority:1" json:"organization_id"`

	// Koneksi WhatsApp yang menerima pesan.
	WhatsAppConnectionID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_conversation_customer,priority:2" json:"whatsapp_connection_id"`

	// Identitas pelanggan WhatsApp.
	CustomerPhone string `gorm:"type:varchar(30);not null;uniqueIndex:idx_conversation_customer,priority:3" json:"customer_phone"`

	CustomerName *string `gorm:"type:varchar(150)" json:"customer_name,omitempty"`

	// Informasi pesan terakhir.
	LastMessageAt      *time.Time `json:"last_message_at,omitempty"`
	LastMessagePreview *string    `gorm:"type:varchar(255)" json:"last_message_preview,omitempty"`

	// Jumlah pesan masuk yang belum dibaca.
	UnreadCount int `gorm:"not null;default:0" json:"unread_count"`

	// Status conversation.
	Status ConversationStatus `gorm:"type:varchar(20);not null;default:open" json:"status"`

	// Relasi organisasi.
	Organization Organization `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`

	// Relasi koneksi WhatsApp.
	WhatsAppConnection WhatsAppConnection `gorm:"foreignKey:WhatsAppConnectionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`

	// Relasi pesan.
	Messages []Message `gorm:"foreignKey:ConversationID" json:"-"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (c *Conversation) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}

	if c.Status == "" {
		c.Status = ConversationOpen
	}

	return nil
}
