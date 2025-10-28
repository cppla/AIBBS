package utils

import (
	"image/color"

	"github.com/mojocn/base64Captcha"
)

var (
	captchaStore = base64Captcha.DefaultMemStore
)

// GenerateCaptcha creates a captcha and returns (id, dataURI) for frontend to display.
func GenerateCaptcha() (string, string, error) {
	// Use a simple digit captcha: width 120, height 40, length 5
	driver := base64Captcha.NewDriverDigit(40, 120, 5, 0.7, 80)
	// Optional style: for string captcha
	_ = color.RGBA{R: 240, G: 240, B: 240, A: 255}
	c := base64Captcha.NewCaptcha(driver, captchaStore)
	id, b64, _, err := c.Generate()
	return id, b64, err
}

// VerifyCaptcha verifies the provided answer; it consumes the captcha on success.
func VerifyCaptcha(id, answer string) bool {
	if id == "" || answer == "" {
		return false
	}
	return captchaStore.Verify(id, answer, true)
}

// VerifyCaptchaNoConsume verifies without consuming the stored answer.
func VerifyCaptchaNoConsume(id, answer string) bool {
	if id == "" || answer == "" {
		return false
	}
	return captchaStore.Verify(id, answer, false)
}
