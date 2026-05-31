package sgin

import "github.com/gin-gonic/gin"

// Decision 表示权限判断结果。
// 使用结构体而不是 bool，可以把拒绝原因稳定地传回给调用方。
type Decision struct {
	Allowed bool
	Code    string
	Message string
}

const (
	// ErrCodeAuthenticationRequired 表示请求没有提供认证信息。
	ErrCodeAuthenticationRequired = "authentication_required"
	// ErrCodeInvalidAuthorization 表示 Authorization 请求头格式错误。
	ErrCodeInvalidAuthorization = "invalid_authorization"
	// ErrCodeInvalidToken 表示 token 无效或对应用户不存在。
	ErrCodeInvalidToken = "invalid_token"
	// ErrCodeTokenExpired 表示 access token 已过期。
	ErrCodeTokenExpired = "token_expired"
	// ErrCodeInvalidTokenType 表示 token 类型不符合当前接口要求。
	ErrCodeInvalidTokenType = "invalid_token_type"
	// ErrCodeAccountDisabled 表示账号已禁用。
	ErrCodeAccountDisabled = "account_disabled"
	// ErrCodeInvalidCredentials 表示登录用户名或密码错误。
	ErrCodeInvalidCredentials = "invalid_credentials"
	// ErrCodeInvalidRefreshToken 表示 refresh token 无效或已被轮换。
	ErrCodeInvalidRefreshToken = "invalid_refresh_token"
	// ErrCodeRefreshTokenExpired 表示 refresh token 已过期。
	ErrCodeRefreshTokenExpired = "refresh_token_expired"
	// ErrCodePermissionDenied 表示通用权限拒绝。
	ErrCodePermissionDenied = "permission_denied"
	// ErrCodeAdminRequired 表示需要管理员权限。
	ErrCodeAdminRequired = "admin_required"
	// ErrCodeGroupRequired 表示需要命中指定用户组。
	ErrCodeGroupRequired = "group_required"
	// ErrCodeRoleRequired 表示需要命中指定角色。
	ErrCodeRoleRequired = "role_required"
	// ErrCodeRoutePermissionRequired 表示当前用户缺少路由要求的权限点。
	ErrCodeRoutePermissionRequired = "route_permission_required"
	// ErrCodeRoutePermissionNotConfigured 表示当前路由没有配置动态权限。
	ErrCodeRoutePermissionNotConfigured = "route_permission_not_configured"
)

// Allow 返回允许访问的判断结果。
func Allow() Decision {
	return Decision{Allowed: true}
}

// Deny 返回拒绝访问的判断结果。
func Deny(code, message string) Decision {
	return Decision{
		Allowed: false,
		Code:    code,
		Message: message,
	}
}

// Permission 是请求级权限接口。
type Permission interface {
	HasPermission(c *gin.Context, action string) Decision
}

// ObjectPermission 是对象级权限接口。
type ObjectPermission[T any] interface {
	HasObjectPermission(c *gin.Context, action string, obj *T) Decision
}

// QueryPermission 用于收窄列表查询范围。
type QueryPermission[T any] interface {
	ScopeQuery(c *gin.Context, action string, q Query) Query
}
