package sgin

import "github.com/gin-gonic/gin"

// AllowAny 允许所有请求通过。
type AllowAny struct{}

// HasPermission 总是允许。
func (AllowAny) HasPermission(c *gin.Context, action string) Decision {
	return Allow()
}

// IsAuthenticated 要求 Gin Context 中已经写入 user。
type IsAuthenticated struct{}

// HasPermission 检查 c.Get("user") 是否存在。
func (IsAuthenticated) HasPermission(c *gin.Context, action string) Decision {
	if user, exists := c.Get("user"); exists && user != nil {
		return Allow()
	}
	return Deny(ErrCodeAuthenticationRequired, "authentication required")
}

// AdminUser 是 IsAdmin 支持的最小用户接口。
type AdminUser interface {
	IsAdmin() bool
}

// IsAdmin 要求当前用户实现 AdminUser 且 IsAdmin 返回 true。
type IsAdmin struct{}

// HasPermission 检查 Gin Context 中的 user 是否为管理员。
func (IsAdmin) HasPermission(c *gin.Context, action string) Decision {
	user, exists := c.Get("user")
	if !exists || user == nil {
		return Deny(ErrCodeAuthenticationRequired, "authentication required")
	}
	admin, ok := user.(AdminUser)
	if !ok || !admin.IsAdmin() {
		return Deny(ErrCodeAdminRequired, "admin permission required")
	}
	return Allow()
}

// ReadOnly 只允许列表和详情读取动作。
type ReadOnly struct{}

// HasPermission 判断 action 是否为只读动作。
func (ReadOnly) HasPermission(c *gin.Context, action string) Decision {
	if action == ActionList || action == ActionRetrieve {
		return Allow()
	}
	return Deny(ErrCodePermissionDenied, "write action is not allowed")
}

// denyPermission 是内部使用的固定拒绝权限。
// 当配置引用了尚未注册的权限名称时，用它给出明确错误，而不是静默放行。
type denyPermission struct {
	decision Decision
}

// HasPermission 返回固定拒绝结果。
func (p denyPermission) HasPermission(c *gin.Context, action string) Decision {
	return p.decision
}
