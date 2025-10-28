package controllers

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/cppla/aibbs/models"
	"github.com/cppla/aibbs/utils"
)

// StatsController provides forum statistics such as counts and daily active users.
type StatsController struct {
	db *gorm.DB
}

// NewStatsController creates a new StatsController instance.
func NewStatsController(db *gorm.DB) *StatsController {
	return &StatsController{db: db}
}

// GetStats returns aggregate statistics for the forum.
func (s *StatsController) GetStats(ctx *gin.Context) {
	var userCount int64
	var postCount int64
	var commentCount int64
	var dailyActive int64

	if err := s.db.Model(&models.User{}).Count(&userCount).Error; err != nil {
		// Fallback to 0 instead of failing the whole endpoint
		userCount = 0
	}

	if err := s.db.Model(&models.Post{}).Count(&postCount).Error; err != nil {
		postCount = 0
	}

	if err := s.db.Model(&models.Comment{}).Count(&commentCount).Error; err != nil {
		commentCount = 0
	}

	// Daily active (PV-based): sum of today's page views across all paths
	// Use string date equality to avoid timezone/type mismatches with DATE column
	today := time.Now().In(time.Local).Format("2006-01-02")
	if err := s.db.Model(&models.PageView{}).
		Where("date = ?", today).
		Select("COALESCE(SUM(count),0)").
		Scan(&dailyActive).Error; err != nil {
		dailyActive = 0
	}

	utils.Success(ctx, gin.H{
		"user_count":         userCount,
		"post_count":         postCount,
		"comment_count":      commentCount,
		"daily_active_count": dailyActive,
	})
}

// GetPostStats returns PV and comments count for a given post id.
func (s *StatsController) GetPostStats(ctx *gin.Context) {
	id := ctx.Param("id")
	// PV: sum over all dates and path formats including pretty URL /post-<id>-<page>
	var pv int64
	path1 := "/api/v1/posts/" + id
	path2 := "/posts/" + id
	prettyLike := "%/post-" + id + "-%" // matches /post-123-1, /post-123-2, etc.
	if err := s.db.Model(&models.PageView{}).
		Where("path IN ? OR path LIKE ?", []string{path1, path2}, prettyLike).
		Select("COALESCE(SUM(count),0)").
		Scan(&pv).Error; err != nil {
		pv = 0
	}

	var commentsCount int64
	if err := s.db.Model(&models.Comment{}).Where("post_id = ?", id).Count(&commentsCount).Error; err != nil {
		commentsCount = 0
	}

	utils.Success(ctx, gin.H{
		"pv":             pv,
		"comments_count": commentsCount,
	})
}
