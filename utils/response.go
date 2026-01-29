package utils

import (
	"github.com/gin-gonic/gin"
)

type APIResponse struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message"`
}

func SendSuccessResponse(c *gin.Context, code int, data interface{}, message string) {
	c.JSON(code, APIResponse{
		Code:    code,
		Data:    data,
		Message: message,
	})
}

func SendErrorResponse(c *gin.Context, code int, message string) {
	c.JSON(code, APIResponse{
		Code:    code,
		Message: message,
	})
}
