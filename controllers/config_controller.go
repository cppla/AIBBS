package controllers

import (
	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/utils"
	"github.com/gin-gonic/gin"
)

// ConfigController serves dynamic, environment-driven UI configuration.
type ConfigController struct{}

func NewConfigController() *ConfigController { return &ConfigController{} }

// GetFooter returns footer configuration loaded from environment with defaults.
func (c *ConfigController) GetFooter(ctx *gin.Context) {
	cfg := config.Get()
	utils.Success(ctx, gin.H{
		"col1": gin.H{
			"title": cfg.FooterCol1Title,
			"html":  cfg.FooterCol1HTML,
		},
		"col2": gin.H{
			"title": cfg.FooterCol2Title,
			"links": []gin.H{
				{"name": cfg.FooterLink1Name, "url": cfg.FooterLink1URL},
				{"name": cfg.FooterLink2Name, "url": cfg.FooterLink2URL},
				{"name": cfg.FooterLink3Name, "url": cfg.FooterLink3URL},
			},
		},
		"col3": gin.H{
			"title":         cfg.FooterCol3Title,
			"telegram_url":  cfg.FooterTelegramURL,
			"email_link":    cfg.FooterEmailLink,
			"broadcast_url": cfg.FooterBroadcastURL,
		},
	})
}

// GetNotice returns announcement/notice content configured via config.
func (c *ConfigController) GetNotice(ctx *gin.Context) {
	cfg := config.Get()
	utils.Success(ctx, gin.H{
		"title": cfg.NoticeTitle,
		"html":  cfg.NoticeHTML,
	})
}
