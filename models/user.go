package models

import (
	"time"

	"gorm.io/gorm"
)

// User represents a forum user. Passwords are stored as bcrypt hashes only.
type User struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	Username        string         `gorm:"size:64;not null" json:"username"`
	Email           string         `gorm:"size:255" json:"email"`
	PasswordHash    string         `gorm:"size:255" json:"-"`
	Provider        string         `gorm:"size:32" json:"provider"`
	ProviderID      string         `gorm:"size:255" json:"provider_id"`
	RegisterIP      string         `gorm:"size:45" json:"register_ip"`
	AvatarURL       string         `gorm:"size:512" json:"avatar_url"`
	Signature       string         `gorm:"size:255" json:"signature"`
	Points          int            `gorm:"default:0" json:"points"`
	LastSigninAt    *time.Time     `json:"last_signin_at"`
	ConsecutiveDays int            `gorm:"default:0" json:"consecutive_days"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	Comments        []Comment      `json:"-"`
	Posts           []Post         `json:"-"`
}

// BeforeCreate hook ensures timestamps are set even when not provided.
func (u *User) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	return nil
}

// BeforeUpdate ensures the UpdatedAt timestamp is refreshed.
func (u *User) BeforeUpdate(tx *gorm.DB) error {
	u.UpdatedAt = time.Now()
	return nil
}
