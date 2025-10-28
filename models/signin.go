package models

import "time"

// SignIn stores daily sign-in records for users.
type SignIn struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"index;not null" json:"user_id"`
	SigninDate     time.Time `gorm:"index;not null" json:"signin_date"`
	PointsAwarded  int       `json:"points_awarded"`
	StreakAchieved int       `json:"streak_achieved"`
	CreatedAt      time.Time `json:"created_at"`
}
