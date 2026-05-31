package sgin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 框架内置错误，用于 Repository、配置加载和权限流程之间传递统一语义。
var (
	ErrNotFound        = errors.New("not found")
	ErrForbidden       = errors.New("forbidden")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrBadRequest      = errors.New("bad request")
	ErrConfigNotFound  = errors.New("config not found")
	ErrRepositoryUnset = errors.New("repository is not set")
)

// HandleError 将常见错误转换为统一 JSON 响应。
func HandleError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	status := statusFromError(err)
	JSON(c, status, Fail(status, http.StatusText(status), err.Error()))
}

// statusFromError 根据错误类型选择 HTTP 状态码。
func statusFromError(err error) int {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// writeDecisionDenied 将权限拒绝结果输出为统一响应。
func writeDecisionDenied(c *gin.Context, decision Decision) {
	status := http.StatusForbidden
	if isAuthenticationErrorCode(decision.Code) {
		status = http.StatusUnauthorized
	}
	message := decision.Message
	if message == "" {
		message = http.StatusText(status)
	}
	JSON(c, status, Fail(status, message, decision.Code))
}

func isAuthenticationErrorCode(code string) bool {
	switch code {
	case ErrCodeAuthenticationRequired,
		ErrCodeInvalidAuthorization,
		ErrCodeInvalidToken,
		ErrCodeTokenExpired,
		ErrCodeInvalidTokenType,
		ErrCodeAccountDisabled,
		"unauthorized",
		"not_authenticated":
		return true
	default:
		return false
	}
}
