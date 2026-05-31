package sgin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// APIView 注册单个 method + path 路由。
// 未设置 Handler 时，会按 Method 和 Path 推断默认 CRUD 动作并复用 ModelViewSet 逻辑。
type APIView[T any, ID comparable] struct {
	Method string
	Path   string

	Repository Repository[T, ID]
	Serializer Serializer[T]

	Auth        []string
	Middlewares []gin.HandlerFunc
	Handler     gin.HandlerFunc

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
func (v *APIView[T, ID]) setApp(app *App) {
	v.app = app
}

// Register 注册单个路由。
func (v *APIView[T, ID]) Register(r gin.IRouter) {
	method := normalizeMethodKey(v.Method)
	path := normalizeBasePath(v.Path)
	validateAuthConfig(v.Auth)

	handler := v.Handler
	if handler == nil {
		handler = v.defaultHandler(method, path)
	}
	r.Handle(strings.ToUpper(method), path, v.routeHandlers(method, handler)...)
}

func (v *APIView[T, ID]) routeHandlers(method string, handler gin.HandlerFunc) []gin.HandlerFunc {
	handlers := make([]gin.HandlerFunc, 0, 1+len(v.Middlewares)+1)
	if authRequired(v.Auth, method) {
		if v.app == nil {
			panic("sgin: APIView Auth requires App.Register")
		}
		handlers = append(handlers, v.app.JWTAuth())
	}
	handlers = appendHandlers(handlers, v.Middlewares...)
	handlers = append(handlers, handler)
	return handlers
}

func (v *APIView[T, ID]) defaultHandler(method string, path string) gin.HandlerFunc {
	model := v.model()
	hasID := routeHasID(path)

	switch method {
	case strings.ToLower(http.MethodGet):
		if hasID {
			return model.Retrieve
		}
		return model.List
	case strings.ToLower(http.MethodPost):
		if hasID {
			panic(fmt.Sprintf("sgin: APIView %s %s cannot use default create with :id path", method, path))
		}
		return model.Create
	case strings.ToLower(http.MethodPut):
		requireIDPath(method, path, hasID)
		return model.Update
	case strings.ToLower(http.MethodPatch):
		requireIDPath(method, path, hasID)
		return model.PartialUpdate
	case strings.ToLower(http.MethodDelete):
		requireIDPath(method, path, hasID)
		return model.Destroy
	default:
		panic(fmt.Sprintf("sgin: unsupported HTTP method %q", method))
	}
}

func (v *APIView[T, ID]) model() *ModelViewSet[T, ID] {
	return &ModelViewSet[T, ID]{
		BasePath:          v.Path,
		Repository:        v.Repository,
		Serializer:        v.Serializer,
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

func routeHasID(path string) bool {
	return strings.Contains(path, ":id")
}

func requireIDPath(method string, path string, hasID bool) {
	if !hasID {
		panic(fmt.Sprintf("sgin: APIView %s %s requires :id path", method, path))
	}
}
