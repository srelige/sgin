package sgin

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ReadOnlyModelViewSet 只注册列表和详情两个只读路由。
type ReadOnlyModelViewSet[T any, ID comparable] struct {
	BasePath string

	Repository     Repository[T, ID]
	Serializer     Serializer[T]
	Auth           []string
	AllowAnonymous []string

	Middlewares       []gin.HandlerFunc
	ActionMiddlewares map[string][]gin.HandlerFunc
	Handlers          map[string]gin.HandlerFunc

	Permissions       []Permission
	ActionPermissions map[string][]Permission
	ObjectPermissions []ObjectPermission[T]
	QueryPermissions  []QueryPermission[T]

	Pagination        Pagination
	DisablePagination bool
	Filters           []FilterBackend

	LookupField     string
	SearchFields    []string
	OrderingFields  []string
	DefaultOrdering string
	FilterFields    []string

	app *App
}

// setApp 接收 App 注入。
func (v *ReadOnlyModelViewSet[T, ID]) setApp(app *App) {
	v.app = app
}

// Path 返回 ViewSet 的基础路径。
func (v *ReadOnlyModelViewSet[T, ID]) Path() string {
	return normalizeBasePath(v.BasePath)
}

// Register 只注册 GET /resources 和 GET /resources/:id。
func (v *ReadOnlyModelViewSet[T, ID]) Register(r gin.IRouter) {
	base := v.Path()
	model := v.model()
	model.validateReadOnlyRouteOptions()
	model.markAnonymousRoute(http.MethodGet, base, ActionList)
	r.GET(base, model.routeHandlers(http.MethodGet, ActionList, model.List)...)
	item := joinIDPath(base)
	model.markAnonymousRoute(http.MethodGet, item, ActionRetrieve)
	r.GET(item, model.routeHandlers(http.MethodGet, ActionRetrieve, model.Retrieve)...)
}

// List 处理只读列表请求。
func (v *ReadOnlyModelViewSet[T, ID]) List(c *gin.Context) {
	v.model().List(c)
}

// Retrieve 处理只读详情请求。
func (v *ReadOnlyModelViewSet[T, ID]) Retrieve(c *gin.Context) {
	v.model().Retrieve(c)
}

// model 将只读 ViewSet 复用为完整 ModelViewSet 的读逻辑。
func (v *ReadOnlyModelViewSet[T, ID]) model() *ModelViewSet[T, ID] {
	return &ModelViewSet[T, ID]{
		BasePath:          v.BasePath,
		Repository:        v.Repository,
		Serializer:        v.Serializer,
		Auth:              v.Auth,
		AllowAnonymous:    v.AllowAnonymous,
		Middlewares:       v.Middlewares,
		ActionMiddlewares: v.ActionMiddlewares,
		Handlers:          v.Handlers,
		Permissions:       v.Permissions,
		ActionPermissions: v.ActionPermissions,
		ObjectPermissions: v.ObjectPermissions,
		QueryPermissions:  v.QueryPermissions,
		Pagination:        v.Pagination,
		DisablePagination: v.DisablePagination,
		Filters:           v.Filters,
		LookupField:       v.LookupField,
		SearchFields:      v.SearchFields,
		OrderingFields:    v.OrderingFields,
		DefaultOrdering:   v.DefaultOrdering,
		FilterFields:      v.FilterFields,
		app:               v.app,
	}
}
