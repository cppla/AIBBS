package models

import "time"

// UploadedFile records locally stored uploaded files for timed cleanup.
type UploadedFile struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	FilePath  string    `gorm:"size:1024;not null" json:"file_path"` // absolute or relative filesystem path
	URL       string    `gorm:"size:1024;not null" json:"url"`       // public URL like /static/uploads/...
	ExpireAt  time.Time `gorm:"index" json:"expire_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
