package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/cppla/aibbs/models"
)

// PageViewRecorder records page views per day and path.
func PageViewRecorder(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Only record successful page views (2xx) for GET requests.
		if c.Request.Method != "GET" {
			return
		}
		status := c.Writer.Status()
		if status < 200 || status >= 400 {
			return
		}

		// Use actual request path to distinguish resources like /api/v1/posts/123
		path := c.Request.URL.Path
		// Ignore non-content endpoints to avoid skewing PV (e.g., health, stats, API, static assets)
		if path == "/health" || strings.Contains(path, "/stats") || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/static/") {
			return
		}

		// Use local midnight to align with DATE column
		now := time.Now().In(time.Local)
		localMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

		// Atomic upsert to avoid duplicate key errors under concurrency
		_ = db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "date"}, {Name: "path"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"count": gorm.Expr("count + 1"), "updated_at": time.Now()}),
		}).Create(&models.PageView{Date: localMidnight, Path: path, Count: 1}).Error
	}
}
