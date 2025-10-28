package controllers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/models"
	"github.com/cppla/aibbs/utils"
)

// SignInController handles daily sign-in endpoints.
type SignInController struct {
	db *gorm.DB
}

var errAlreadySignedIn = errors.New("already signed in today")

// NewSignInController creates a new controller instance.
func NewSignInController(db *gorm.DB) *SignInController {
	return &SignInController{db: db}
}

// DailySignIn records a daily sign-in and updates streak / rewards.
func (s *SignInController) DailySignIn(ctx *gin.Context) {
	userID, ok := getUserID(ctx)
	if !ok {
		utils.Error(ctx, http.StatusUnauthorized, 40110, "unauthorized")
		return
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.Add(24 * time.Hour)

	var existing models.SignIn
	if err := s.db.Where("user_id = ? AND signin_date >= ? AND signin_date < ?", userID, todayStart, tomorrowStart).First(&existing).Error; err == nil {
		utils.Error(ctx, http.StatusBadRequest, 40030, "already signed in today")
		return
	}

	cfg := config.Get()
	reward := cfg.SigninRewardPoints

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}

		var lastSignIn models.SignIn
		err := tx.Where("user_id = ?", userID).Order("signin_date DESC").First(&lastSignIn).Error

		streak := 1
		if err == nil {
			if isSameDay(lastSignIn.SigninDate, todayStart) {
				return errAlreadySignedIn
			}
			if isYesterday(lastSignIn.SigninDate, todayStart) {
				streak = lastSignIn.StreakAchieved + 1
			}
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		record := models.SignIn{
			UserID:         userID,
			SigninDate:     now,
			PointsAwarded:  reward,
			StreakAchieved: streak,
		}

		if err := tx.Create(&record).Error; err != nil {
			return err
		}

		user.Points += reward
		user.ConsecutiveDays = streak
		user.LastSigninAt = &record.SigninDate

		return tx.Save(&user).Error
	})

	if err != nil {
		if errors.Is(err, errAlreadySignedIn) {
			utils.Error(ctx, http.StatusBadRequest, 40030, err.Error())
			return
		}
		utils.Error(ctx, http.StatusInternalServerError, 50030, "failed to record sign-in")
		return
	}

	utils.Success(ctx, gin.H{
		"message":        "sign-in successful",
		"points_awarded": reward,
	})
}

// SignInStatus returns the user's streak and last sign-in time.
func (s *SignInController) SignInStatus(ctx *gin.Context) {
	userID, ok := getUserID(ctx)
	if !ok {
		utils.Error(ctx, http.StatusUnauthorized, 40110, "unauthorized")
		return
	}

	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.Error(ctx, http.StatusNotFound, 40410, "user not found")
			return
		}
		utils.Error(ctx, http.StatusInternalServerError, 50031, "failed to load user")
		return
	}

	utils.Success(ctx, gin.H{
		"points":           user.Points,
		"consecutive_days": user.ConsecutiveDays,
		"last_signin_at":   user.LastSigninAt,
	})
}

func isSameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func isYesterday(last, today time.Time) bool {
	yesterday := today.Add(-24 * time.Hour)
	return last.Year() == yesterday.Year() && last.YearDay() == yesterday.YearDay()
}
