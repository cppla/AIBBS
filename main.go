package main

import (
	"time"

	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/models"
	"github.com/cppla/aibbs/routes"
	"github.com/cppla/aibbs/utils"
)

func main() {
	cfg := config.Load()

	// Initialize logger early
	if err := utils.InitLogger(cfg); err != nil {
		panic(err)
	}

	// Auto-migrate all models including SignIn and PageView
	db := config.InitDatabase(&models.User{}, &models.Post{}, &models.Comment{}, &models.SignIn{}, &models.PageView{}, &models.UploadedFile{})

	r := routes.SetupRouter(db)

	// Start background cleanup for expired uploads (best-effort)
	utils.StartUploadCleaner(5 * time.Minute)

	utils.Sugar.Infof("Starting server on port %s (graceful)", cfg.AppPort)
	if err := utils.GraceServer(":"+cfg.AppPort, r); err != nil {
		utils.Sugar.Fatalf("server stopped with error: %v", err)
	}
}
