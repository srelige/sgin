package sgin

import (
	"embed"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

//go:embed ui/admin.html
var adminUIFS embed.FS

type adminCreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  *bool  `json:"enabled"`
}

type adminCreateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type adminCreateRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type adminCreatePermissionRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type adminCreateRoutePermissionRequest struct {
	Method         string `json:"method"`
	Path           string `json:"path"`
	PermissionCode string `json:"permission_code"`
	Enabled        *bool  `json:"enabled"`
}

type adminBindGroupRequest struct {
	GroupID uint `json:"group_id"`
}

type adminBindRoleRequest struct {
	RoleID uint `json:"role_id"`
}

type adminBindPermissionRequest struct {
	PermissionID uint `json:"permission_id"`
}

type adminStateResponse struct {
	Users            []UserAccount      `json:"users"`
	Groups           []AccessGroup      `json:"groups"`
	Roles            []AccessRole       `json:"roles"`
	Permissions      []AccessPermission `json:"permissions"`
	RoutePermissions []RoutePermission  `json:"route_permissions"`
}

func registerAdminRoutes(app *App) {
	if app == nil || !app.config.Admin.Enabled {
		return
	}
	base := normalizeAdminPath(app.config.Admin.Path)
	apiBase := strings.TrimRight(base, "/") + "/api"
	if base == "/" {
		apiBase = "/api"
	}

	app.GET(base, app.serveAdminUI)
	api := app.Group(apiBase, app.JWTAuth(), app.LoadAccess(), app.RequireAnyGroup("admin"))
	api.GET("/state", app.handleAdminState)
	api.POST("/users", app.handleAdminCreateUser)
	api.PATCH("/users/:id/enabled", app.handleAdminSetUserEnabled)
	api.POST("/groups", app.handleAdminCreateGroup)
	api.POST("/roles", app.handleAdminCreateRole)
	api.POST("/permissions", app.handleAdminCreatePermission)
	api.POST("/route-permissions", app.handleAdminCreateRoutePermission)
	api.POST("/users/:id/groups", app.handleAdminBindUserGroup)
	api.POST("/users/:id/roles", app.handleAdminBindUserRole)
	api.POST("/groups/:id/roles", app.handleAdminBindGroupRole)
	api.POST("/roles/:id/permissions", app.handleAdminBindRolePermission)
}

func normalizeAdminPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/sgin-admin"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "/"
	}
	return path
}

func (a *App) serveAdminUI(c *gin.Context) {
	data, err := adminUIFS.ReadFile("ui/admin.html")
	if err != nil {
		JSON(c, http.StatusInternalServerError, Fail(http.StatusInternalServerError, "admin UI file not found", err.Error()))
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func (a *App) handleAdminState(c *gin.Context) {
	db, err := a.DB()
	if err != nil {
		HandleError(c, err)
		return
	}
	var resp adminStateResponse
	if err := db.Preload("Groups").Preload("Roles").Find(&resp.Users).Error; err != nil {
		HandleError(c, err)
		return
	}
	if err := db.Preload("Roles").Find(&resp.Groups).Error; err != nil {
		HandleError(c, err)
		return
	}
	if err := db.Preload("Permissions").Find(&resp.Roles).Error; err != nil {
		HandleError(c, err)
		return
	}
	if err := db.Find(&resp.Permissions).Error; err != nil {
		HandleError(c, err)
		return
	}
	if err := db.Find(&resp.RoutePermissions).Error; err != nil {
		HandleError(c, err)
		return
	}
	JSON(c, http.StatusOK, OK(resp))
}

func (a *App) handleAdminCreateUser(c *gin.Context) {
	var req adminCreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleError(c, fmt.Errorf("%w: %v", ErrBadRequest, err))
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		HandleError(c, fmt.Errorf("%w: username and password are required", ErrBadRequest))
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		HandleError(c, err)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	user := UserAccount{Username: req.Username, PasswordHash: string(hash), Enabled: enabled}
	db, err := a.DB()
	if err != nil {
		HandleError(c, err)
		return
	}
	if err := db.Select("Username", "PasswordHash", "Admin", "Enabled").Create(&user).Error; err != nil {
		HandleError(c, err)
		return
	}
	if !enabled {
		if err := db.Model(&user).Update("enabled", false).Error; err != nil {
			HandleError(c, err)
			return
		}
		user.Enabled = false
	}
	JSON(c, http.StatusCreated, Created(user))
}

func (a *App) handleAdminSetUserEnabled(c *gin.Context) {
	id, ok := adminUintParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleError(c, fmt.Errorf("%w: %v", ErrBadRequest, err))
		return
	}
	db, err := a.DB()
	if err != nil {
		HandleError(c, err)
		return
	}
	if err := db.Model(&UserAccount{}).Where("id = ?", id).Update("enabled", req.Enabled).Error; err != nil {
		HandleError(c, err)
		return
	}
	JSON(c, http.StatusOK, OK(map[string]any{"enabled": req.Enabled}))
}

func (a *App) handleAdminCreateGroup(c *gin.Context) {
	var req adminCreateGroupRequest
	if !bindAdminJSON(c, &req) {
		return
	}
	group := AccessGroup{Name: strings.TrimSpace(req.Name), Description: req.Description}
	if group.Name == "" {
		HandleError(c, fmt.Errorf("%w: group name is required", ErrBadRequest))
		return
	}
	a.adminCreate(c, &group)
}

func (a *App) handleAdminCreateRole(c *gin.Context) {
	var req adminCreateRoleRequest
	if !bindAdminJSON(c, &req) {
		return
	}
	role := AccessRole{Name: strings.TrimSpace(req.Name), Description: req.Description}
	if role.Name == "" {
		HandleError(c, fmt.Errorf("%w: role name is required", ErrBadRequest))
		return
	}
	a.adminCreate(c, &role)
}

func (a *App) handleAdminCreatePermission(c *gin.Context) {
	var req adminCreatePermissionRequest
	if !bindAdminJSON(c, &req) {
		return
	}
	permission := AccessPermission{Code: strings.TrimSpace(req.Code), Name: req.Name, Description: req.Description}
	if permission.Code == "" {
		HandleError(c, fmt.Errorf("%w: permission code is required", ErrBadRequest))
		return
	}
	a.adminCreate(c, &permission)
}

func (a *App) handleAdminCreateRoutePermission(c *gin.Context) {
	var req adminCreateRoutePermissionRequest
	if !bindAdminJSON(c, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	method, ok := normalizeAdminRouteMethod(c, req.Method)
	if !ok {
		return
	}
	route := RoutePermission{Method: strings.ToUpper(method), Path: strings.TrimSpace(req.Path), PermissionCode: strings.TrimSpace(req.PermissionCode), Enabled: enabled}
	if route.Method == "" || route.Path == "" || route.PermissionCode == "" {
		HandleError(c, fmt.Errorf("%w: method, path and permission_code are required", ErrBadRequest))
		return
	}
	a.adminCreate(c, &route)
}

func (a *App) handleAdminBindUserGroup(c *gin.Context) {
	userID, ok := adminUintParam(c, "id")
	if !ok {
		return
	}
	var req adminBindGroupRequest
	if !bindAdminJSON(c, &req) {
		return
	}
	a.adminCreate(c, &UserGroup{UserAccountID: userID, AccessGroupID: req.GroupID})
}

func (a *App) handleAdminBindUserRole(c *gin.Context) {
	userID, ok := adminUintParam(c, "id")
	if !ok {
		return
	}
	var req adminBindRoleRequest
	if !bindAdminJSON(c, &req) {
		return
	}
	a.adminCreate(c, &UserRole{UserAccountID: userID, AccessRoleID: req.RoleID})
}

func (a *App) handleAdminBindGroupRole(c *gin.Context) {
	groupID, ok := adminUintParam(c, "id")
	if !ok {
		return
	}
	var req adminBindRoleRequest
	if !bindAdminJSON(c, &req) {
		return
	}
	a.adminCreate(c, &GroupRole{AccessGroupID: groupID, AccessRoleID: req.RoleID})
}

func (a *App) handleAdminBindRolePermission(c *gin.Context) {
	roleID, ok := adminUintParam(c, "id")
	if !ok {
		return
	}
	var req adminBindPermissionRequest
	if !bindAdminJSON(c, &req) {
		return
	}
	a.adminCreate(c, &RolePermission{AccessRoleID: roleID, AccessPermissionID: req.PermissionID})
}

func normalizeAdminRouteMethod(c *gin.Context, method string) (value string, ok bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			HandleError(c, fmt.Errorf("%w: unsupported HTTP method", ErrBadRequest))
			value = ""
			ok = false
		}
	}()
	return normalizeMethodKey(method), true
}
func bindAdminJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		HandleError(c, fmt.Errorf("%w: %v", ErrBadRequest, err))
		return false
	}
	return true
}

func (a *App) adminCreate(c *gin.Context, value any) {
	db, err := a.DB()
	if err != nil {
		HandleError(c, err)
		return
	}
	if err := db.Create(value).Error; err != nil {
		HandleError(c, err)
		return
	}
	JSON(c, http.StatusCreated, Created(value))
}

func adminUintParam(c *gin.Context, name string) (uint, bool) {
	parsed, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		HandleError(c, err)
		return 0, false
	}
	return uint(parsed), true
}
