
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	CognitoSub   string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"-"`
	Email        string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"email"`
	Name         string         `gorm:"type:varchar(100);not null" json:"name"`
	Status       string         `gorm:"type:varchar(20);not null;default:active" json:"status"`
	Memberships  []Membership   `gorm:"foreignKey:UserID" json:"-"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
