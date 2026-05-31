package sgin

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// ModelViewSet 提供标准 RESTful CRUD 路由。
// T 是业务模型类型，ID 是路由 :id 对应的主键类型。
type ModelViewSet[T any, ID comparable] struct {
	BasePath string

	Repository Repository[T, ID]
	Serializer Serializer[T]
	Auth       []string

	Middlewares       []gin.HandlerFunc
	ActionMiddlewares map[string][]gin.HandlerFunc
	Handlers          map[string]gin.HandlerFunc
	ExtraActions      []ExtraAction

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

	// app 由 App.Register 注入，用于读取 App 配置和默认数据库。
	app *App

	repoOnce    sync.Once
	repoInitErr error
}

// extraActionRoutes 将额外动作拆成可直接注册和需要 dispatcher 分发的两类。
type extraActionRoutes struct {
	direct     []registeredExtraAction
	dispatcher map[string][]registeredExtraAction
}

// registeredExtraAction 保存规范化后的额外动作路由信息。
type registeredExtraAction struct {
	path      string
	routePath string
	action    ExtraAction
}

const selectedExtraActionContextKey = "sgin.selected_extra_action"

// setApp 接收 App 注入。
func (v *ModelViewSet[T, ID]) setApp(app *App) {
	v.app = app
}

// Path 返回 ViewSet 的基础路径。
func (v *ModelViewSet[T, ID]) Path() string {
	return normalizeBasePath(v.BasePath)
}

// Register 注册六个标准 CRUD 路由。
func (v *ModelViewSet[T, ID]) Register(r gin.IRouter) {
	base := v.Path()
	item := joinIDPath(base)

	v.validateRouteOptions()
	extraActions := v.prepareExtraActions(base)

	r.GET(base, v.routeHandlers(http.MethodGet, v.List)...)
	r.POST(base, v.routeHandlers(http.MethodPost, v.Create)...)
	r.GET(item, v.routeHandlersWithCollectionDispatcher(http.MethodGet, v.Retrieve, extraActions.dispatcher["get"])...)
	r.PUT(item, v.routeHandlersWithCollectionDispatcher(http.MethodPut, v.Update, extraActions.dispatcher["put"])...)
	r.PATCH(item, v.routeHandlersWithCollectionDispatcher(http.MethodPatch, v.PartialUpdate, extraActions.dispatcher["patch"])...)
	r.DELETE(item, v.routeHandlersWithCollectionDispatcher(http.MethodDelete, v.Destroy, extraActions.dispatcher["delete"])...)
	for _, action := range extraActions.direct {
		method := normalizeMethodKey(action.action.Method)
		r.Handle(strings.ToUpper(method), action.routePath, v.extraActionHandlers(method, action.action)...)
	}
}

func (v *ModelViewSet[T, ID]) routeHandlers(method string, defaultHandler gin.HandlerFunc) []gin.HandlerFunc {
	key := normalizeMethodKey(method)
	finalHandler := v.resolveHandler(key, defaultHandler)

	handlers := make([]gin.HandlerFunc, 0, 1+len(v.Middlewares)+len(v.resolveActionMiddlewares(key))+1)
	if authRequired(v.Auth, key) {
		if v.app == nil {
			panic("sgin: ModelViewSet Auth requires App.Register")
		}
		handlers = append(handlers, v.app.JWTAuth())
	}
	handlers = appendHandlers(handlers, v.Middlewares...)
	handlers = appendHandlers(handlers, v.resolveActionMiddlewares(key)...)
	handlers = append(handlers, finalHandler)
	return handlers
}

// routeHandlersWithCollectionDispatcher 让 GET/PUT/PATCH/DELETE 集合动作与 /resources/:id 共用入口。
func (v *ModelViewSet[T, ID]) routeHandlersWithCollectionDispatcher(method string, defaultHandler gin.HandlerFunc, actions []registeredExtraAction) []gin.HandlerFunc {
	if len(actions) == 0 {
		return v.routeHandlers(method, defaultHandler)
	}

	key := normalizeMethodKey(method)
	finalHandler := v.resolveHandler(key, defaultHandler)
	actionMiddlewares := v.resolveActionMiddlewares(key)

	capacity := 1 + len(v.Middlewares) + len(actionMiddlewares) + 1
	for _, action := range actions {
		capacity += len(action.action.Middlewares)
	}
	handlers := make([]gin.HandlerFunc, 0, capacity)
	if authRequired(v.Auth, key) {
		if v.app == nil {
			panic("sgin: ModelViewSet Auth requires App.Register")
		}
		handlers = append(handlers, v.app.JWTAuth())
	}
	handlers = append(handlers, selectCollectionExtraAction(actions))
	handlers = appendHandlers(handlers, v.Middlewares...)
	for _, middleware := range actionMiddlewares {
		handlers = append(handlers, onlyForDefaultAction(middleware))
	}
	for _, action := range actions {
		for _, middleware := range action.action.Middlewares {
			handlers = append(handlers, onlyForExtraAction(action.path, middleware))
		}
	}
	handlers = append(handlers, dispatchCollectionExtraAction(finalHandler))
	return handlers
}

func (v *ModelViewSet[T, ID]) extraActionHandlers(method string, action ExtraAction) []gin.HandlerFunc {
	handlers := make([]gin.HandlerFunc, 0, 1+len(v.Middlewares)+len(action.Middlewares)+1)
	if authRequired(v.Auth, method) {
		if v.app == nil {
			panic("sgin: ModelViewSet Auth requires App.Register")
		}
		handlers = append(handlers, v.app.JWTAuth())
	}
	handlers = appendHandlers(handlers, v.Middlewares...)
	handlers = appendHandlers(handlers, action.Middlewares...)
	handlers = append(handlers, action.Handler)
	return handlers
}

// selectCollectionExtraAction 在详情路由入口识别集合动作，并覆盖动态权限检查使用的业务路径。
func selectCollectionExtraAction(actions []registeredExtraAction) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := normalizeExtraActionPath(c.Param("id"))
		for _, action := range actions {
			if action.path == path {
				c.Set(selectedExtraActionContextKey, action)
				setRoutePermissionPath(c, action.routePath)
				return
			}
		}
	}
}

// onlyForDefaultAction 限制标准 CRUD action middleware 不影响 dispatcher 选中的集合动作。
func onlyForDefaultAction(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := selectedExtraAction(c); ok {
			return
		}
		handler(c)
	}
}

// onlyForExtraAction 只在 dispatcher 选中指定集合动作时执行动作级 middleware。
func onlyForExtraAction(path string, handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		action, ok := selectedExtraAction(c)
		if !ok || action.path != path {
			return
		}
		handler(c)
	}
}

// dispatchCollectionExtraAction 在集合动作和默认详情 handler 之间做最终分发。
func dispatchCollectionExtraAction(defaultHandler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		action, ok := selectedExtraAction(c)
		if ok {
			action.action.Handler(c)
			return
		}
		defaultHandler(c)
	}
}

// selectedExtraAction 读取 dispatcher 选中的集合动作。
func selectedExtraAction(c *gin.Context) (registeredExtraAction, bool) {
	value, exists := c.Get(selectedExtraActionContextKey)
	if !exists {
		return registeredExtraAction{}, false
	}
	action, ok := value.(registeredExtraAction)
	return action, ok
}

func (v *ModelViewSet[T, ID]) resolveHandler(method string, defaultHandler gin.HandlerFunc) gin.HandlerFunc {
	for key, handler := range v.Handlers {
		if normalizeMethodKey(key) == method && handler != nil {
			return handler
		}
	}
	return defaultHandler
}

func (v *ModelViewSet[T, ID]) resolveActionMiddlewares(method string) []gin.HandlerFunc {
	var handlers []gin.HandlerFunc
	for key, values := range v.ActionMiddlewares {
		if normalizeMethodKey(key) == method {
			handlers = append(handlers, values...)
		}
	}
	return handlers
}

func (v *ModelViewSet[T, ID]) validateRouteOptions() {
	validateAuthConfig(v.Auth)
	validateActionMiddlewareConfig(v.ActionMiddlewares)
	validateHandlerConfig(v.Handlers)
	validateExtraActions(v.ExtraActions)
}

// prepareExtraActions 预处理额外动作，避免直接注册会和 Gin 通配路由冲突的集合动作。
func (v *ModelViewSet[T, ID]) prepareExtraActions(base string) extraActionRoutes {
	routes := extraActionRoutes{dispatcher: map[string][]registeredExtraAction{}}
	seen := map[string]struct{}{}
	for _, action := range v.ExtraActions {
		method := normalizeMethodKey(action.Method)
		path := normalizeExtraActionPath(action.Path)
		routePath := extraActionPath(base, action)
		key := method + " " + routePath
		if _, ok := seen[key]; ok {
			panic(fmt.Sprintf("sgin: duplicate ExtraAction route %s %s", strings.ToUpper(method), routePath))
		}
		seen[key] = struct{}{}

		registered := registeredExtraAction{path: path, routePath: routePath, action: action}
		if shouldDispatchCollectionExtraAction(method, action) {
			v.validateCollectionExtraAction(method, path, routePath)
			routes.dispatcher[method] = append(routes.dispatcher[method], registered)
			continue
		}
		routes.direct = append(routes.direct, registered)
	}
	return routes
}

// validateCollectionExtraAction 阻止集合动作路径和当前 ID 类型产生语义冲突。
func (v *ModelViewSet[T, ID]) validateCollectionExtraAction(method, path, routePath string) {
	if strings.Contains(path, "/") || strings.ContainsAny(path, ":*") {
		panic(fmt.Sprintf("sgin: collection ExtraAction %s %s must be a single literal path segment", strings.ToUpper(method), routePath))
	}
	if _, err := ParseID[ID](path); err == nil {
		panic(fmt.Sprintf("sgin: collection ExtraAction %s %s conflicts with detail route id", strings.ToUpper(method), routePath))
	}
}

// shouldDispatchCollectionExtraAction 判断集合动作是否需要和详情路由共用 dispatcher。
func shouldDispatchCollectionExtraAction(method string, action ExtraAction) bool {
	return !action.Detail && hasDetailRouteMethod(method)
}

// hasDetailRouteMethod 判断 method 是否已经存在标准详情路由。
func hasDetailRouteMethod(method string) bool {
	switch method {
	case "get", "put", "patch", "delete":
		return true
	default:
		return false
	}
}

func authRequired(auth []string, method string) bool {
	if len(auth) == 0 {
		return false
	}

	validateAuthConfig(auth)
	method = normalizeMethodKey(method)
	for _, item := range auth {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "all" || value == method {
			return true
		}
	}
	return false
}

func validateAuthConfig(auth []string) {
	for _, item := range auth {
		value := strings.ToLower(strings.TrimSpace(item))
		switch {
		case value == "all":
			if len(auth) != 1 {
				panic(`sgin: Auth "all" must be used alone`)
			}
		case isSupportedMethodKey(value):
		default:
			panic(fmt.Sprintf("sgin: unsupported Auth method %q", item))
		}
	}
}

func validateActionMiddlewareConfig(items map[string][]gin.HandlerFunc) {
	for key := range items {
		normalizeMethodKey(key)
	}
}

func validateHandlerConfig(items map[string]gin.HandlerFunc) {
	for key := range items {
		normalizeMethodKey(key)
	}
}

func validateExtraActions(items []ExtraAction) {
	for _, action := range items {
		normalizeMethodKey(action.Method)
		if normalizeExtraActionPath(action.Path) == "" {
			panic("sgin: ExtraAction Path is required")
		}
		if action.Handler == nil {
			panic("sgin: ExtraAction Handler is required")
		}
	}
}

func normalizeMethodKey(method string) string {
	key := strings.ToLower(strings.TrimSpace(method))
	if !isSupportedMethodKey(key) {
		panic(fmt.Sprintf("sgin: unsupported HTTP method %q", method))
	}
	return key
}

func isSupportedMethodKey(method string) bool {
	switch method {
	case "get", "post", "put", "patch", "delete":
		return true
	default:
		return false
	}
}

func appendHandlers(target []gin.HandlerFunc, handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	for _, handler := range handlers {
		if handler != nil {
			target = append(target, handler)
		}
	}
	return target
}

// List 处理 GET /resources。
func (v *ModelViewSet[T, ID]) List(c *gin.Context) {
	if !v.checkPermissions(c, ActionList) {
		return
	}
	if !v.ensureRepository(c) {
		return
	}

	paginationEnabled := v.paginationEnabled()
	q := Query{Filters: map[string]string{}}
	if paginationEnabled {
		q = v.pagination().Parse(c, v.config())
	}
	for _, filter := range v.filters() {
		q = filter.Apply(c, q)
	}
	for _, perm := range v.QueryPermissions {
		q = perm.ScopeQuery(c, ActionList, q)
	}
	q = v.applyDefaultOrdering(q)

	items, err := v.Repository.List(c.Request.Context(), q)
	if err != nil {
		HandleError(c, err)
		return
	}

	serializer := v.serializer()
	data := serializer.ToListResponse(c, items)
	if paginationEnabled {
		total, err := v.count(c, q, int64(len(items)))
		if err != nil {
			HandleError(c, err)
			return
		}
		data = v.pagination().Wrap(c, data, total, q)
	}
	JSON(c, http.StatusOK, OK(data))
}

// Retrieve 处理 GET /resources/:id。
func (v *ModelViewSet[T, ID]) Retrieve(c *gin.Context) {
	if !v.checkPermissions(c, ActionRetrieve) {
		return
	}
	obj, ok := v.findObject(c, ActionRetrieve)
	if !ok {
		return
	}
	JSON(c, http.StatusOK, OK(v.serializer().ToRetrieveResponse(c, obj)))
}

// Create 处理 POST /resources。
func (v *ModelViewSet[T, ID]) Create(c *gin.Context) {
	if !v.checkPermissions(c, ActionCreate) {
		return
	}
	if !v.ensureRepository(c) {
		return
	}

	obj, err := v.serializer().BindCreate(c)
	if err != nil {
		HandleError(c, fmt.Errorf("%w: %v", ErrBadRequest, err))
		return
	}
	if err := v.Repository.Create(c.Request.Context(), obj); err != nil {
		HandleError(c, err)
		return
	}
	JSON(c, http.StatusCreated, Created(v.serializer().ToRetrieveResponse(c, obj)))
}

// Update 处理 PUT /resources/:id。
func (v *ModelViewSet[T, ID]) Update(c *gin.Context) {
	v.updateObject(c, ActionUpdate)
}

// PartialUpdate 处理 PATCH /resources/:id。
// MVP 阶段与 Update 共用逻辑，后续可在 Serializer 中扩展字段级 patch。
func (v *ModelViewSet[T, ID]) PartialUpdate(c *gin.Context) {
	v.updateObject(c, ActionPartialUpdate)
}

// Destroy 处理 DELETE /resources/:id。
func (v *ModelViewSet[T, ID]) Destroy(c *gin.Context) {
	if !v.checkPermissions(c, ActionDestroy) {
		return
	}
	id, obj, ok := v.findObjectWithID(c, ActionDestroy)
	if !ok {
		return
	}
	if !v.checkObjectPermissions(c, ActionDestroy, obj) {
		return
	}
	if err := v.Repository.Delete(c.Request.Context(), id); err != nil {
		HandleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// updateObject 实现 PUT/PATCH 的公共流程。
func (v *ModelViewSet[T, ID]) updateObject(c *gin.Context, action string) {
	if !v.checkPermissions(c, action) {
		return
	}
	obj, ok := v.findObject(c, action)
	if !ok {
		return
	}
	updated, err := v.serializer().BindUpdate(c, obj)
	if err != nil {
		HandleError(c, fmt.Errorf("%w: %v", ErrBadRequest, err))
		return
	}
	if err := v.Repository.Update(c.Request.Context(), updated); err != nil {
		HandleError(c, err)
		return
	}
	data := v.serializer().ToUpdateResponse(c, updated)
	if action == ActionPartialUpdate {
		data = v.serializer().ToPartialUpdateResponse(c, updated)
	}
	JSON(c, http.StatusOK, OK(data))
}

// findObject 从路由参数解析 ID 并读取对象。
func (v *ModelViewSet[T, ID]) findObject(c *gin.Context, action string) (*T, bool) {
	_, obj, ok := v.findObjectWithID(c, action)
	return obj, ok
}

// findObjectWithID 返回 ID 和对象，供 Destroy 等需要 ID 的流程使用。
func (v *ModelViewSet[T, ID]) findObjectWithID(c *gin.Context, action string) (ID, *T, bool) {
	var zero ID
	if !v.ensureRepository(c) {
		return zero, nil, false
	}
	id, err := ParseID[ID](c.Param("id"))
	if err != nil {
		HandleError(c, err)
		return zero, nil, false
	}
	obj, err := v.Repository.Find(c.Request.Context(), id)
	if err != nil {
		HandleError(c, err)
		return zero, nil, false
	}
	if !v.checkObjectPermissions(c, action, obj) {
		return zero, nil, false
	}
	return id, obj, true
}

// ensureRepository 确认 Repository 已配置。
func (v *ModelViewSet[T, ID]) ensureRepository(c *gin.Context) bool {
	if v.Repository == nil {
		v.repoOnce.Do(func() {
			v.Repository, v.repoInitErr = v.defaultRepository()
		})
		if v.repoInitErr != nil {
			HandleError(c, v.repoInitErr)
			return false
		}
	}
	return true
}

// defaultRepository 创建 ModelViewSet 的默认 GORM Repository。
func (v *ModelViewSet[T, ID]) defaultRepository() (Repository[T, ID], error) {
	if v.app == nil {
		return nil, ErrRepositoryUnset
	}
	db, err := v.app.DB()
	if err != nil {
		return nil, err
	}
	repo := &GORMRepository[T, ID]{
		DB:             db,
		LookupField:    v.LookupField,
		SearchFields:   v.SearchFields,
		OrderingFields: v.OrderingFields,
		FilterFields:   v.FilterFields,
	}
	if v.config().Database.AutoMigrate {
		if err := repo.AutoMigrate(); err != nil {
			return nil, err
		}
	}
	return repo, nil
}

// applyDefaultOrdering 在请求未指定 ordering 时写入 ViewSet 默认排序。
func (v *ModelViewSet[T, ID]) applyDefaultOrdering(q Query) Query {
	if strings.TrimSpace(q.OrderBy) == "" && strings.TrimSpace(v.DefaultOrdering) != "" {
		q.OrderBy = strings.TrimSpace(v.DefaultOrdering)
	}
	return q
}

// count 返回过滤后、分页前的总数；自定义 Repository 未实现 CountRepository 时回退为当前页数量。
func (v *ModelViewSet[T, ID]) count(c *gin.Context, q Query, fallback int64) (int64, error) {
	counter, ok := v.Repository.(CountRepository[T, ID])
	if !ok {
		return fallback, nil
	}
	return counter.Count(c.Request.Context(), q)
}

// serializer 返回用户 Serializer 或默认 JSON Serializer。
func (v *ModelViewSet[T, ID]) serializer() Serializer[T] {
	if v.Serializer != nil {
		return v.Serializer
	}
	return DefaultSerializer[T]{}
}

// pagination 返回用户分页器或默认 page/page_size 分页器。
func (v *ModelViewSet[T, ID]) pagination() Pagination {
	if v.Pagination != nil {
		return v.Pagination
	}
	return PageNumberPagination{}
}

// paginationEnabled 判断当前列表接口是否启用分页。
func (v *ModelViewSet[T, ID]) paginationEnabled() bool {
	if v.DisablePagination {
		return false
	}
	return v.config().REST.Pagination
}

// filters 返回过滤器列表。
// nil 表示使用框架默认过滤器；空切片表示用户显式关闭过滤器。
func (v *ModelViewSet[T, ID]) filters() []FilterBackend {
	if v.Filters == nil {
		return []FilterBackend{SearchFilter{}, OrderingFilter{}, FieldFilter{}}
	}
	return v.Filters
}

// config 返回 App 配置；未通过 App.Register 注册时使用默认配置兜底。
func (v *ModelViewSet[T, ID]) config() Config {
	if v.app != nil {
		return v.app.config
	}
	return DefaultConfig()
}

// checkPermissions 按 ViewSet 权限、Action 权限顺序执行请求级权限检查。
func (v *ModelViewSet[T, ID]) checkPermissions(c *gin.Context, action string) bool {
	for _, perm := range v.requestPermissions(action) {
		if perm == nil {
			continue
		}
		decision := perm.HasPermission(c, action)
		if !decision.Allowed {
			writeDecisionDenied(c, decision)
			return false
		}
	}
	return true
}

// requestPermissions 合并显式请求级权限。
func (v *ModelViewSet[T, ID]) requestPermissions(action string) []Permission {
	var perms []Permission
	perms = append(perms, v.Permissions...)
	if len(v.ActionPermissions) > 0 {
		perms = append(perms, v.ActionPermissions[action]...)
	}
	return perms
}

// checkObjectPermissions 执行对象级权限检查。
func (v *ModelViewSet[T, ID]) checkObjectPermissions(c *gin.Context, action string, obj *T) bool {
	for _, perm := range v.ObjectPermissions {
		if perm == nil {
			continue
		}
		decision := perm.HasObjectPermission(c, action, obj)
		if !decision.Allowed {
			writeDecisionDenied(c, decision)
			return false
		}
	}
	return true
}

// normalizeBasePath 统一基础路由格式。
func normalizeBasePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}

// joinIDPath 拼接详情路由。
func joinIDPath(base string) string {
	if base == "/" {
		return "/:id"
	}
	return base + "/:id"
}

func extraActionPath(base string, action ExtraAction) string {
	path := normalizeExtraActionPath(action.Path)
	if action.Detail {
		return joinPaths(joinIDPath(base), path)
	}
	return joinPaths(base, path)
}

func normalizeExtraActionPath(path string) string {
	return strings.Trim(strings.TrimSpace(path), "/")
}

func joinPaths(base, path string) string {
	if base == "/" {
		return "/" + path
	}
	return base + "/" + path
}
