package sgin

import "github.com/gin-gonic/gin"

const (
	// ActionList 对应 GET /resources。
	ActionList = "list"
	// ActionRetrieve 对应 GET /resources/:id。
	ActionRetrieve = "retrieve"
	// ActionCreate 对应 POST /resources。
	ActionCreate = "create"
	// ActionUpdate 对应 PUT /resources/:id。
	ActionUpdate = "update"
	// ActionPartialUpdate 对应 PATCH /resources/:id。
	ActionPartialUpdate = "partial_update"
	// ActionDestroy 对应 DELETE /resources/:id。
	ActionDestroy = "destroy"
)

// HandlerFunc 是返回统一 Response 的处理函数签名。
type HandlerFunc func(c *gin.Context) Response

// View 描述一个更底层的单路由视图。
type View interface {
	Method() string
	Path() string
	Handle(c *gin.Context) Response
}

// ViewSet 描述可注册到 App 的路由集合或单路由视图。
type ViewSet interface {
	Register(r gin.IRouter)
}

// ExtraAction 描述注册到 ModelViewSet 资源路径下的额外动作路由。
type ExtraAction struct {
	// Method 是动作使用的 HTTP method。
	Method string
	// Path 是相对资源路径的动作路径。
	Path string
	// Detail 为 true 时动作挂到 /resources/:id/{path}，否则挂到 /resources/{path}。
	Detail         bool
	RequireAuth    bool
	AllowAnonymous bool
	// Middlewares 只作用于当前额外动作。
	Middlewares []gin.HandlerFunc
	// Handler 是当前额外动作最终执行的 Gin handler。
	Handler gin.HandlerFunc
}
