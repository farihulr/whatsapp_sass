
package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MembershipRole string

const (
	RoleOwner MembershipRole = "owner"
	RoleAdmin MembershipRole = "admin"
	RoleAgent MembershipRole = "agent"
)

type Membership struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	UserID         uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_membership_user_org" json:"user_id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_membership_user_org;index" json:"organization_id"`
	Role           MembershipRole `gorm:"type:varchar(20);not null;default:agent;check:chk_membership_role,role IN ('owner','admin','agent')" json:"role"`
	User           User           `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	Organization   Organization   `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

func (m *Membership) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}

	if m.Role == "" {
		m.Role = RoleAgent
	}

	return m.validateRole()
}

func (m *Membership) BeforeUpdate(tx *gorm.DB) error {
	return m.validateRole()
}

func (m *Membership) validateRole() error {
	switch m.Role {
	case RoleOwner, RoleAdmin, RoleAgent:
		return nil
	default:
		return fmt.Errorf("role tidak valid: %q", m.Role)
	}
}
