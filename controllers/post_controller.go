package controllers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/middleware"
	"github.com/cppla/aibbs/models"
	"github.com/cppla/aibbs/utils"
)

// PostController manages CRUD operations for posts and comments.
type PostController struct {
	db *gorm.DB
}

// NewPostController creates a new PostController instance.
func NewPostController(db *gorm.DB) *PostController {
	return &PostController{db: db}
}

// CreatePost allows authenticated users to create new posts.
func (p *PostController) CreatePost(ctx *gin.Context) {
	var req struct {
		Title       string `json:"title" binding:"required,min=1"`
		Content     string `json:"content" binding:"required"`
		Category    string `json:"category"`
		Attachments string `json:"attachments"` // JSON array of URLs
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40020, "invalid request payload")
		return
	}

	title := utils.Sanitize(strings.TrimSpace(req.Title))
	if title == "" {
		utils.Error(ctx, http.StatusBadRequest, 40021, "title cannot be empty")
		return
	}

	content := utils.Sanitize(req.Content)
	category := req.Category
	if category == "" {
		category = "综合"
	}
	// Validate category
	validCategories := []string{"综合", "评测", "技术", "线报", "推广", "交易"}
	isValid := false
	for _, c := range validCategories {
		if category == c {
			isValid = true
			break
		}
	}
	if !isValid {
		utils.Error(ctx, http.StatusBadRequest, 40022, "invalid category")
		return
	}

	userID, ok := getUserID(ctx)
	if !ok {
		utils.Error(ctx, http.StatusUnauthorized, 40110, "unauthorized")
		return
	}

	post := models.Post{
		UserID:      userID,
		Title:       title,
		Content:     content,
		Category:    category,
		Attachments: req.Attachments,
	}

	if err := p.db.Create(&post).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50020, "failed to create post")
		return
	}

	// Invalidate lists cache (homepage and categories)
	utils.InvalidateByPrefix("cache:posts:list:")
	// Invalidate user posts cache for this author
	utils.InvalidateByPrefix("cache:user:" + strconv.Itoa(int(userID)) + ":posts:")

	utils.Success(ctx, gin.H{"post": post})
}

// ListPosts returns paginated posts including author information.
func (p *PostController) ListPosts(ctx *gin.Context) {
	page, pageSize := parsePagination(ctx.Query("page"), ctx.Query("page_size"))
	search := strings.TrimSpace(ctx.Query("search"))
	category := strings.TrimSpace(ctx.Query("category"))

	// Cache homepage/category lists when no search term to avoid cache key explosion
	if search == "" {
		cacheKey := fmt.Sprintf("cache:posts:list:cat=%s:page=%d:size=%d", category, page, pageSize)
		if b, ok := utils.CacheGetBytes(cacheKey); ok {
			ctx.Data(200, "application/json", b)
			return
		}
	}

	var posts []models.Post
	var total int64

	query := p.db.Preload("User").Order("created_at DESC")
	if search != "" {
		query = query.Where("title LIKE ? OR content LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if err := query.Model(&models.Post{}).Count(&total).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50021, "failed to count posts")
		return
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&posts).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50022, "failed to list posts")
		return
	}

	// 兼容说明：JSON 中包含 author（关联的 User），前端也兼容 user 字段读取。

	payload := gin.H{
		"items": posts,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": int((total + int64(pageSize) - 1) / int64(pageSize)),
		},
	}
	if search == "" {
		cacheKey := fmt.Sprintf("cache:posts:list:cat=%s:page=%d:size=%d", category, page, pageSize)
		// Wrap in standard response and cache
		wrapper := struct {
			Code    int         `json:"code"`
			Message string      `json:"message"`
			Data    interface{} `json:"data"`
		}{Code: 0, Message: "success", Data: payload}
		utils.CacheSetJSON(cacheKey, wrapper, time.Hour)
	}
	utils.Success(ctx, payload)
}

// GetPost returns a single post with comments.
func (p *PostController) GetPost(ctx *gin.Context) {
	postID := ctx.Param("id")

	// Try cache first
	if b, ok := utils.CacheGetBytes("cache:post:detail:" + postID); ok {
		ctx.Data(200, "application/json", b)
		return
	}

	var post models.Post
	if err := p.db.Preload("User").First(&post, postID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.Error(ctx, http.StatusNotFound, 40401, "post not found")
			return
		}
		utils.Error(ctx, http.StatusInternalServerError, 50023, "failed to load post")
		return
	}

	// Load comments separately for better error handling
	var comments []models.Comment
	if err := p.db.Model(&post).Association("Comments").Find(&comments); err != nil {
		// Log the error but don't fail the whole request
		fmt.Println("Failed to load comments:", err)
	} else {
		post.Comments = comments
	}

	// Manually load users for comments if comments were loaded
	if len(post.Comments) > 0 {
		var userIDs []uint
		for _, c := range post.Comments {
			userIDs = append(userIDs, c.UserID)
		}
		// Remove duplicates
		userIDs = utils.UniqueUint(userIDs)

		var users []models.User
		if err := p.db.Find(&users, userIDs).Error; err == nil {
			userMap := make(map[uint]models.User)
			for _, u := range users {
				userMap[u.ID] = u
			}
			for i := range post.Comments {
				if user, ok := userMap[post.Comments[i].UserID]; ok {
					post.Comments[i].User = user
				}
			}
		} else {
			fmt.Println("Failed to load users for comments:", err)
		}
	}

	payload := gin.H{"post": post}
	wrapper := struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}{Code: 0, Message: "success", Data: payload}
	utils.CacheSetJSON("cache:post:detail:"+postID, wrapper, time.Hour)
	utils.Success(ctx, payload)
}

// ListMyPosts returns posts created by the authenticated user.
func (p *PostController) ListMyPosts(ctx *gin.Context) {
	userID, ok := getUserID(ctx)
	if !ok {
		utils.Error(ctx, http.StatusUnauthorized, 40110, "unauthorized")
		return
	}
	page, pageSize := parsePagination(ctx.Query("page"), ctx.Query("page_size"))
	var posts []models.Post
	var total int64
	q := p.db.Where("user_id = ?", userID).Preload("User").Order("created_at DESC")
	if err := q.Model(&models.Post{}).Count(&total).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50027, "failed to count user posts")
		return
	}
	if err := q.Offset((page - 1) * pageSize).Limit(pageSize).Find(&posts).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50028, "failed to list user posts")
		return
	}
	utils.Success(ctx, gin.H{
		"items": posts,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": int((total + int64(pageSize) - 1) / int64(pageSize)),
		},
	})
}

// ListUserPosts returns posts created by a specific user (public)
func (p *PostController) ListUserPosts(ctx *gin.Context) {
	userID := strings.TrimSpace(ctx.Param("id"))
	if userID == "" {
		utils.Error(ctx, http.StatusBadRequest, 40060, "missing user id")
		return
	}
	page, pageSize := parsePagination(ctx.Query("page"), ctx.Query("page_size"))
	// try cache first
	cacheKey := fmt.Sprintf("cache:user:%s:posts:page=%d:size=%d", userID, page, pageSize)
	if b, ok := utils.CacheGetBytes(cacheKey); ok {
		ctx.Data(200, "application/json", b)
		return
	}
	var posts []models.Post
	var total int64
	q := p.db.Where("user_id = ?", userID).Preload("User").Order("created_at DESC")
	if err := q.Model(&models.Post{}).Count(&total).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50060, "failed to count user posts")
		return
	}
	if err := q.Offset((page - 1) * pageSize).Limit(pageSize).Find(&posts).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50061, "failed to list user posts")
		return
	}
	payload := gin.H{
		"items": posts,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": int((total + int64(pageSize) - 1) / int64(pageSize)),
		},
	}
	wrapper := struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}{Code: 0, Message: "success", Data: payload}
	utils.CacheSetJSON(cacheKey, wrapper, time.Hour)
	utils.Success(ctx, payload)
}

// CreateComment allows authenticated users to comment on posts.
func (p *PostController) CreateComment(ctx *gin.Context) {
	var req struct {
		Content string `json:"content" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40022, "invalid request payload")
		return
	}

	content := utils.Sanitize(req.Content)
	if content == "" {
		utils.Error(ctx, http.StatusBadRequest, 40023, "content cannot be empty")
		return
	}

	postID := ctx.Param("id")
	var post models.Post
	if err := p.db.First(&post, postID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.Error(ctx, http.StatusNotFound, 40402, "post not found")
			return
		}
		utils.Error(ctx, http.StatusInternalServerError, 50024, "failed to load post")
		return
	}

	userID, ok := getUserID(ctx)
	if !ok {
		utils.Error(ctx, http.StatusUnauthorized, 40110, "unauthorized")
		return
	}

	comment := models.Comment{
		PostID:  post.ID,
		UserID:  userID,
		Content: content,
	}

	if err := p.db.Create(&comment).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50025, "failed to create comment")
		return
	}

	if err := p.db.Preload("User").First(&comment, comment.ID).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50026, "failed to load comment")
		return
	}

	// Invalidate post detail cache on new comment
	utils.InvalidateByPrefix("cache:post:detail:" + strconv.Itoa(int(post.ID)))
	// Invalidate user posts cache for post author (comments don't change user list, skip)

	utils.Success(ctx, gin.H{"comment": comment})
}

// DeleteComment allows the comment owner or admin to delete a comment
func (p *PostController) DeleteComment(ctx *gin.Context) {
	// comment id from path
	cid := strings.TrimSpace(ctx.Param("commentId"))
	if cid == "" {
		utils.Error(ctx, http.StatusBadRequest, 40070, "missing comment id")
		return
	}
	var cmt models.Comment
	if err := p.db.First(&cmt, cid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.Error(ctx, http.StatusNotFound, 40420, "comment not found")
			return
		}
		utils.Error(ctx, http.StatusInternalServerError, 50070, "failed to load comment")
		return
	}

	uid, ok := getUserID(ctx)
	if !ok {
		utils.Error(ctx, http.StatusUnauthorized, 40120, "unauthorized")
		return
	}
	if cmt.UserID != uid && !isAdmin(ctx) {
		utils.Error(ctx, http.StatusForbidden, 40320, "you can only delete your own comment")
		return
	}
	if err := p.db.Delete(&cmt).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50071, "failed to delete comment")
		return
	}
	// Invalidate post cache
	utils.InvalidateByPrefix("cache:post:detail:" + strconv.Itoa(int(cmt.PostID)))
	utils.Success(ctx, gin.H{"message": "comment deleted"})
}

// UpdatePost allows the author to update their post.
func (p *PostController) UpdatePost(ctx *gin.Context) {
	var req struct {
		Title       string `json:"title" binding:"required,min=1"`
		Content     string `json:"content" binding:"required"`
		Category    string `json:"category"`
		Attachments string `json:"attachments"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40024, "invalid request payload")
		return
	}

	title := utils.Sanitize(strings.TrimSpace(req.Title))
	if title == "" {
		utils.Error(ctx, http.StatusBadRequest, 40025, "title cannot be empty")
		return
	}

	content := utils.Sanitize(req.Content)
	category := req.Category
	if category == "" {
		category = "综合"
	}
	// Validate category
	validCategories := []string{"综合", "评测", "技术", "线报", "推广", "交易"}
	isValid := false
	for _, c := range validCategories {
		if category == c {
			isValid = true
			break
		}
	}
	if !isValid {
		utils.Error(ctx, http.StatusBadRequest, 40026, "invalid category")
		return
	}

	postID := ctx.Param("id")
	var post models.Post
	if err := p.db.First(&post, postID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.Error(ctx, http.StatusNotFound, 40403, "post not found")
			return
		}
		utils.Error(ctx, http.StatusInternalServerError, 50025, "failed to load post")
		return
	}

	userID, ok := getUserID(ctx)
	if !ok {
		utils.Error(ctx, http.StatusUnauthorized, 40111, "unauthorized")
		return
	}

	if post.UserID != userID {
		utils.Error(ctx, http.StatusForbidden, 40301, "you can only update your own posts")
		return
	}

	post.Title = title
	post.Content = content
	post.Category = category
	post.Attachments = req.Attachments
	if err := p.db.Save(&post).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50026, "failed to update post")
		return
	}

	// Invalidate caches for lists and detail
	utils.InvalidateByPrefix("cache:posts:list:")
	utils.InvalidateByPrefix("cache:post:detail:" + postID)
	utils.InvalidateByPrefix("cache:user:" + strconv.Itoa(int(post.UserID)) + ":posts:")

	utils.Success(ctx, gin.H{"post": post})
}

// DeletePost allows the author to delete their post.
func (p *PostController) DeletePost(ctx *gin.Context) {
	postID := ctx.Param("id")
	var post models.Post
	if err := p.db.First(&post, postID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.Error(ctx, http.StatusNotFound, 40404, "post not found")
			return
		}
		utils.Error(ctx, http.StatusInternalServerError, 50027, "failed to load post")
		return
	}

	userID, ok := getUserID(ctx)
	if !ok {
		utils.Error(ctx, http.StatusUnauthorized, 40112, "unauthorized")
		return
	}

	if post.UserID != userID && !isAdmin(ctx) {
		utils.Error(ctx, http.StatusForbidden, 40302, "you can only delete your own posts")
		return
	}

	if err := p.db.Delete(&post).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50028, "failed to delete post")
		return
	}

	// Invalidate lists and detail cache
	utils.InvalidateByPrefix("cache:posts:list:")
	utils.InvalidateByPrefix("cache:post:detail:" + postID)
	utils.InvalidateByPrefix("cache:user:" + strconv.Itoa(int(post.UserID)) + ":posts:")

	utils.Success(ctx, gin.H{"message": "post deleted"})
}

// UploadAttachment handles file uploads for posts.
func (p *PostController) UploadAttachment(ctx *gin.Context) {
	// Auth check
	userID, ok := getUserID(ctx)
	if !ok {
		utils.Error(ctx, http.StatusUnauthorized, 40113, "unauthorized")
		return
	}

	// Accept common field name 'file' or fallback to 'f'
	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		file, header, err = ctx.Request.FormFile("f")
		if err != nil {
			utils.Error(ctx, http.StatusBadRequest, 40030, "no file uploaded")
			return
		}
	}
	defer file.Close()

	// Size limit: 50MB
	const maxSize = 50 * 1024 * 1024
	if header.Size > 0 && header.Size > maxSize {
		utils.Error(ctx, http.StatusBadRequest, 40032, "file size exceeds 50MB")
		return
	}

	now := time.Now()
	year := now.Format("2006")
	month := now.Format("01")
	day := now.Format("02")
	baseDir := filepath.Join(".", "static", "uploads", year, month, day)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50030, "failed to create upload directory")
		return
	}

	// Sanitize filename and ensure uniqueness
	fname := filepath.Base(header.Filename)
	if fname == "." || fname == "" {
		fname = fmt.Sprintf("file_%d", now.UnixNano())
	}
	// prevent collisions: prefix with timestamp and user id
	safeName := fmt.Sprintf("%d_%d_%s", now.UnixNano(), userID, fname)
	dstPath := filepath.Join(baseDir, safeName)

	// Save with limit check
	out, err := os.Create(dstPath)
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50031, "failed to save file")
		return
	}
	defer out.Close()

	// Enforce 50MB by limited reader
	lr := &io.LimitedReader{R: file, N: maxSize + 1}
	written, err := io.Copy(out, lr)
	if err != nil {
		// cleanup
		_ = out.Close()
		_ = os.Remove(dstPath)
		utils.Error(ctx, http.StatusInternalServerError, 50032, "failed to write file")
		return
	}
	if written > maxSize {
		_ = out.Close()
		_ = os.Remove(dstPath)
		utils.Error(ctx, http.StatusBadRequest, 40032, "file size exceeds 50MB")
		return
	}

	// Build public URL
	relURL := fmt.Sprintf("/static/uploads/%s/%s/%s/%s", year, month, day, safeName)
	// Record for persistent cleanup per configuration
	conf := config.Get()
	ttlMinutes := conf.UploadsSelfDestructMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 60
	}
	expireAt := time.Now().Add(time.Duration(ttlMinutes) * time.Minute)
	absPath, _ := filepath.Abs(dstPath)
	// Non-blocking best-effort record; ignore error to not affect upload success
	go func(absPath, url string, exp time.Time) {
		defer func() { _ = recover() }()
		db := config.DB()
		if db != nil {
			_ = db.Create(&models.UploadedFile{FilePath: absPath, URL: url, ExpireAt: exp}).Error
		}
	}(absPath, relURL, expireAt)

	// Also schedule in-memory fallback deletion if enabled
	if conf.UploadsSelfDestructEnabled {
		go func(path string, minutes int) {
			time.Sleep(time.Duration(minutes) * time.Minute)
			_ = os.Remove(path)
		}(absPath, ttlMinutes)
	}

	utils.Success(ctx, gin.H{"url": relURL})
}

func parsePagination(pageStr, sizeStr string) (int, int) {
	page := 1
	pageSize := 10
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
		pageSize = s
	}
	return page, pageSize
}

func getUserID(ctx *gin.Context) (uint, bool) {
	value, exists := ctx.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}

	switch v := value.(type) {
	case uint:
		return v, true
	case int:
		return uint(v), true
	case int64:
		return uint(v), true
	case float64:
		return uint(v), true
	default:
		return 0, false
	}
}

func isAdmin(ctx *gin.Context) bool {
	unameVal, exists := ctx.Get(middleware.ContextUsernameKey)
	if !exists {
		return false
	}
	uname, _ := unameVal.(string)
	if uname == "" {
		return false
	}
	cfg := config.Get()
	for _, u := range cfg.AdminUsernames {
		if strings.EqualFold(strings.TrimSpace(u), uname) {
			return true
		}
	}
	return false
}
