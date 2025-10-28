package utils

import "github.com/microcosm-cc/bluemonday"

var sanitizer = bluemonday.UGCPolicy()

// Sanitize cleans HTML content to prevent XSS attacks.
func Sanitize(input string) string {
	return sanitizer.Sanitize(input)
}
