package utils

import "github.com/gin-gonic/gin"

// JSONResponse defines the uniform structure for API responses.
type JSONResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Respond writes a JSON response with the given status code.
func Respond(ctx *gin.Context, status int, code int, message string, data interface{}) {
	ctx.JSON(status, JSONResponse{
		Code:    code,
		Message: message,
		Data:    data,
	})
}

// Success returns a standard success response.
func Success(ctx *gin.Context, data interface{}) {
	Respond(ctx, 200, 0, "success", data)
}

// Error returns a standard error response.
func Error(ctx *gin.Context, status int, code int, message string) {
	Respond(ctx, status, code, message, nil)
}
