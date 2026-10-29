package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InvitationStatus string

const (
	InvitationPending  InvitationStatus = "pending"
	InvitationAccepted InvitationStatus = "accepted"
	InvitationExpired  InvitationStatus = "expired"
	InvitationRevoked  InvitationStatus = "revoked"
)

type Invitation struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID       `gorm:"type:uuid;not null;index" json:"organization_id"`
	InvitedBy      uuid.UUID       `gorm:"type:uuid;not null" json:"invited_by"`
	Email          string          `gorm:"type:varchar(255);not null;index" json:"email"`
	Role           MembershipRole  `gorm:"type:varchar(20);not null" json:"role"`
	TokenHash      string          `gorm:"type:varchar(64);not null;uniqueIndex" json:"-"`
	Status         InvitationStatus `gorm:"type:varchar(20);not null;default:pending;index" json:"status"`
	ExpiresAt      time.Time       `gorm:"not null" json:"expires_at"`
	AcceptedAt     *time.Time      `json:"accepted_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func (i *Invitation) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}

	if i.Role != RoleAdmin && i.Role != RoleAgent {
		return fmt.Errorf("role undangan harus admin atau agent")
	}

	if i.Status == "" {
		i.Status = InvitationPending
	}

	return nil
}
