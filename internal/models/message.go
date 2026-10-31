package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MessageDirection string

const (
	MessageInbound  MessageDirection = "inbound"
	MessageOutbound MessageDirection = "outbound"
)

type MessageType string

const (
	MessageText        MessageType = "text"
	MessageImage       MessageType = "image"
	MessageAudio       MessageType = "audio"
	MessageVideo       MessageType = "video"
	MessageDocument    MessageType = "document"
	MessageLocation    MessageType = "location"
	MessageInteractive MessageType = "interactive"
	MessageUnknown     MessageType = "unknown"
)

type MessageStatus string

const (
	MessageReceived  MessageStatus = "received"
	MessageAccepted  MessageStatus = "accepted"
	MessageSent      MessageStatus = "sent"
	MessageDelivered MessageStatus = "delivered"
	MessageRead      MessageStatus = "read"
	MessageFailed    MessageStatus = "failed"
)

type Message struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`

	ConversationID uuid.UUID `gorm:"type:uuid;not null;index" json:"conversation_id"`

	// ID pesan dari Meta WhatsApp.
	WhatsAppMessageID *string `gorm:"type:varchar(255);uniqueIndex" json:"whatsapp_message_id,omitempty"`

	Direction MessageDirection `gorm:"type:varchar(20);not null;index" json:"direction"`

	Type MessageType `gorm:"type:varchar(30);not null;default:text" json:"type"`

	Status MessageStatus `gorm:"type:varchar(20);not null;default:received;index" json:"status"`

	// Isi pesan teks. Untuk media, metadata akan disimpan terpisah.
	Body *string `gorm:"type:text" json:"body,omitempty"`

	// Waktu pesan dari WhatsApp.
	MessageAt time.Time `gorm:"not null;index" json:"message_at"`

	// Payload asli webhook untuk keperluan debugging.
	RawPayload []byte `gorm:"type:jsonb" json:"-"`

	Conversation Conversation `gorm:"foreignKey:ConversationID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (m *Message) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}

	if m.Type == "" {
		m.Type = MessageText
	}

	if m.Status == "" {
		m.Status = MessageReceived
	}

	if m.MessageAt.IsZero() {
		m.MessageAt = time.Now().UTC()
	}

	return nil
}
