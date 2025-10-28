package utils

import (
	"log"
	"os"
	"time"

	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/models"
)

// StartUploadCleaner launches a background goroutine that periodically deletes
// expired uploaded files recorded in the database. It is best-effort and logs failures.
func StartUploadCleaner(interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	go func() {
		for {
			// Sleep first to avoid racing immediately at startup
			time.Sleep(interval)
			db := config.DB()
			if db == nil {
				continue
			}
			// obey configuration switch
			c := config.Get()
			if !c.UploadsSelfDestructEnabled {
				continue
			}
			var items []models.UploadedFile
			if err := db.Where("expire_at <= ?", time.Now()).Limit(100).Find(&items).Error; err != nil {
				log.Printf("upload cleaner query failed: %v", err)
				continue
			}
			for _, it := range items {
				if it.FilePath != "" {
					_ = os.Remove(it.FilePath)
				}
				// Remove row regardless of file deletion outcome
				if err := db.Delete(&models.UploadedFile{}, it.ID).Error; err != nil {
					log.Printf("upload cleaner delete row failed: %v", err)
				}
			}
		}
	}()
}
