package sgin

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 是 sgin 的统一响应结构。
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// OK 创建成功响应，业务错误码为 0。
func OK(data any) Response {
	return Response{
		Code:    0,
		Message: "ok",
		Data:    data,
	}
}

// Created 创建资源已创建响应。
func Created(data any) Response {
	return Response{
		Code:    0,
		Message: "created",
		Data:    data,
	}
}

// NoContent 创建无内容响应描述。
// HTTP 204 通常不返回响应体；该函数保留给需要显式构造 Response 的场景。
func NoContent() Response {
	return Response{
		Code:    0,
		Message: "no content",
	}
}

// Fail 创建失败响应。
func Fail(code int, message string, _ ...any) Response {
	return Response{
		Code:    code,
		Message: message,
		Data:    H{},
	}
}

// JSON 使用 Gin 输出统一 JSON。
// 当状态码为 204 时只写状态码，避免发送不符合 HTTP 语义的响应体。
func JSON(c *gin.Context, status int, resp Response) {
	if status == http.StatusNoContent {
		c.Status(status)
		return
	}
	c.JSON(status, resp)
}
