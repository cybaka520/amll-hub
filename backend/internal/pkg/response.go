package pkg

import "github.com/gin-gonic/gin"

// Response 统一 JSON 响应
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// OK 成功响应
func OK(c *gin.Context, data interface{}) {
	c.JSON(200, Response{
		Code:    200,
		Message: "success",
		Data:    data,
	})
}

// OKWithMsg 自定义 message 的成功响应
func OKWithMsg(c *gin.Context, data interface{}, message string) {
	c.JSON(200, Response{
		Code:    200,
		Message: message,
		Data:    data,
	})
}

// Fail 失败响应
func Fail(c *gin.Context, httpCode, code int, message string) {
	c.JSON(httpCode, Response{
		Code:    code,
		Message: message,
	})
}

// BadRequest 400
func BadRequest(c *gin.Context, message string) {
	Fail(c, 400, 400, message)
}

// NotFound 404
func NotFound(c *gin.Context, message string) {
	Fail(c, 404, 404, message)
}

// InternalError 500
func InternalError(c *gin.Context, message string) {
	Fail(c, 500, 500, message)
}
