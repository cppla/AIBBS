package models

import "time"

// PageView stores aggregated page view counts per day and path.
type PageView struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Date      time.Time `gorm:"index:idx_pv_date_path,unique;type:date;not null" json:"date"`
	Path      string    `gorm:"index;index:idx_pv_date_path,unique;size:255;not null" json:"path"`
	Count     int64     `gorm:"not null;default:0" json:"count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
