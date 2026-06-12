package sgin

import (
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	accessContextKey       = "access"
	routePermissionPathKey = "sgin.route_permission_path"
)

// AccessGroup represents an organization or team membership.
type AccessGroup struct {
	ID          uint         `json:"id" gorm:"primaryKey"`
	Name        string       `json:"name" gorm:"uniqueIndex;size:191;not null"`
	Description string       `json:"description"`
	Roles       []AccessRole `json:"roles,omitempty" gorm:"many2many:sgin_group_roles;"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

func (AccessGroup) TableName() string { return "sgin_groups" }

// AccessRole represents a named permission role.
type AccessRole struct {
	ID          uint               `json:"id" gorm:"primaryKey"`
	Name        string             `json:"name" gorm:"uniqueIndex;size:191;not null"`
	Description string             `json:"description"`
	Permissions []AccessPermission `json:"permissions,omitempty" gorm:"many2many:sgin_role_permissions;"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

func (AccessRole) TableName() string { return "sgin_roles" }

// AccessPermission represents a permission code.
type AccessPermission struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"uniqueIndex;size:191;not null"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AccessPermission) TableName() string { return "sgin_permissions" }

// AccessMenu represents a frontend menu or desktop entry controlled by permissions.
type AccessMenu struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	ParentID       *uint        `json:"parent_id" gorm:"index"`
	Name           string       `json:"name" gorm:"uniqueIndex;size:191;not null"`
	Title          string       `json:"title"`
	Path           string       `json:"path"`
	Icon           string       `json:"icon"`
	PermissionCode string       `json:"permission_code" gorm:"index;size:191"`
	Sort           int          `json:"sort"`
	Enabled        bool         `json:"enabled" gorm:"not null;default:true"`
	Children       []AccessMenu `json:"children,omitempty" gorm:"-"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

func (AccessMenu) TableName() string { return "sgin_menus" }

// RoutePermission binds a method/path pair to a permission code.
type RoutePermission struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Method         string    `json:"method" gorm:"uniqueIndex:idx_sgin_route_permission;size:16;not null"`
	Path           string    `json:"path" gorm:"uniqueIndex:idx_sgin_route_permission;size:255;not null"`
	PermissionCode string    `json:"permission_code" gorm:"uniqueIndex:idx_sgin_route_permission;size:191;not null"`
	Enabled        bool      `json:"enabled" gorm:"not null;default:true"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (RoutePermission) TableName() string { return "sgin_route_permissions" }

type UserGroup struct {
	UserAccountID uint `gorm:"primaryKey;column:user_account_id"`
	AccessGroupID uint `gorm:"primaryKey;column:access_group_id"`
}

func (UserGroup) TableName() string { return "sgin_user_groups" }

type UserRole struct {
	UserAccountID uint `gorm:"primaryKey;column:user_account_id"`
	AccessRoleID  uint `gorm:"primaryKey;column:access_role_id"`
}

func (UserRole) TableName() string { return "sgin_user_roles" }

type GroupRole struct {
	AccessGroupID uint `gorm:"primaryKey;column:access_group_id"`
	AccessRoleID  uint `gorm:"primaryKey;column:access_role_id"`
}

func (GroupRole) TableName() string { return "sgin_group_roles" }

type RolePermission struct {
	AccessRoleID       uint `gorm:"primaryKey;column:access_role_id"`
	AccessPermissionID uint `gorm:"primaryKey;column:access_permission_id"`
}

func (RolePermission) TableName() string { return "sgin_role_permissions" }

// AccessInfo is the access context loaded for one request.
type AccessInfo struct {
	UserID      uint     `json:"user_id"`
	Username    string   `json:"username"`
	Groups      []string `json:"groups"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

func (a AccessInfo) HasAnyGroup(groups ...string) bool {
	if containsAny(a.Groups, "admin") {
		return true
	}
	return containsAny(a.Groups, groups...)
}

func (a AccessInfo) HasAnyRole(roles ...string) bool {
	if containsAny(a.Groups, "admin") {
		return true
	}
	return containsAny(a.Roles, roles...)
}

func (a AccessInfo) HasPermission(permission string) bool {
	if containsAny(a.Groups, "admin") {
		return true
	}
	return containsAny(a.Permissions, permission)
}

func (a AccessInfo) IsAdmin() bool {
	return containsAny(a.Groups, "admin")
}

// CurrentAccess returns the access context for the current request.
func CurrentAccess(c *gin.Context) (AccessInfo, bool) {
	value, exists := c.Get(accessContextKey)
	if !exists {
		return AccessInfo{}, false
	}
	access, ok := value.(AccessInfo)
	return access, ok
}

// LoadAccess loads groups, roles, and permissions for the current user.
func (a *App) LoadAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := a.ensureAccess(c); !ok {
			return
		}
		c.Next()
	}
}

// RequireAnyGroup requires the current user to belong to any given group.
func (a *App) RequireAnyGroup(groups ...string) gin.HandlerFunc {
	if len(groups) == 0 {
		panic("sgin: RequireAnyGroup requires at least one group")
	}
	return func(c *gin.Context) {
		access, ok := a.ensureAccess(c)
		if !ok {
			return
		}
		if access.HasAnyGroup(groups...) {
			c.Next()
			return
		}
		writeDecisionDenied(c, Deny(ErrCodeGroupRequired, "group permission required"))
		c.Abort()
	}
}

// RequireAnyRole requires the current user to have any given role.
func (a *App) RequireAnyRole(roles ...string) gin.HandlerFunc {
	if len(roles) == 0 {
		panic("sgin: RequireAnyRole requires at least one role")
	}
	return func(c *gin.Context) {
		access, ok := a.ensureAccess(c)
		if !ok {
			return
		}
		if access.HasAnyRole(roles...) {
			c.Next()
			return
		}
		writeDecisionDenied(c, Deny(ErrCodeRoleRequired, "role permission required"))
		c.Abort()
	}
}

// RequireRoutePermission checks the current method/path against route permissions.
func (a *App) RequireRoutePermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		access, ok := a.ensureAccess(c)
		if !ok {
			return
		}
		if containsAny(access.Groups, "admin") {
			c.Next()
			return
		}

		db, err := a.DB()
		if err != nil {
			HandleError(c, err)
			c.Abort()
			return
		}

		method := strings.ToUpper(c.Request.Method)
		path := routePermissionPath(c)

		var routes []RoutePermission
		if err := db.Where("enabled = ? AND method = ? AND path = ?", true, method, path).Find(&routes).Error; err != nil {
			HandleError(c, err)
			c.Abort()
			return
		}
		if len(routes) == 0 {
			writeDecisionDenied(c, Deny(ErrCodeRoutePermissionNotConfigured, "route permission is not configured"))
			c.Abort()
			return
		}
		for _, route := range routes {
			if access.HasPermission(route.PermissionCode) {
				c.Next()
				return
			}
		}
		writeDecisionDenied(c, Deny(ErrCodeRoutePermissionRequired, "route permission required"))
		c.Abort()
	}
}

// setRoutePermissionPath 覆盖 RequireRoutePermission 使用的业务路由路径。
func setRoutePermissionPath(c *gin.Context, path string) {
	path = strings.TrimSpace(path)
	if path != "" {
		c.Set(routePermissionPathKey, path)
	}
}

// routePermissionPath 返回动态路由权限检查使用的 method/path 里的 path。
func routePermissionPath(c *gin.Context) string {
	if value, ok := c.Get(routePermissionPathKey); ok {
		if path, ok := value.(string); ok && strings.TrimSpace(path) != "" {
			return strings.TrimSpace(path)
		}
	}
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	return path
}

func (a *App) ensureAccess(c *gin.Context) (AccessInfo, bool) {
	if access, ok := CurrentAccess(c); ok {
		return access, true
	}
	access, decision := a.loadAccess(c)
	if !decision.Allowed {
		writeDecisionDenied(c, decision)
		c.Abort()
		return AccessInfo{}, false
	}
	c.Set(accessContextKey, access)
	return access, true
}

func (a *App) loadAccess(c *gin.Context) (AccessInfo, Decision) {
	userID, ok := userIDFromContext(c)
	if !ok {
		return AccessInfo{}, Deny(ErrCodeAuthenticationRequired, "authentication required")
	}
	db, err := a.DB()
	if err != nil {
		return AccessInfo{}, Deny(ErrCodePermissionDenied, "permission check failed")
	}

	user, err := loadUserWithAccess(db, userID)
	if err != nil {
		return AccessInfo{}, Deny(ErrCodeInvalidToken, "invalid access token")
	}
	if !user.Enabled {
		return AccessInfo{}, Deny(ErrCodeAccountDisabled, "account disabled")
	}

	access := buildAccessInfo(user)
	c.Set("user", user)
	return access, Allow()
}

func loadUserWithAccess(db *gorm.DB, userID uint) (UserAccount, error) {
	var user UserAccount
	if err := db.Preload("Groups.Roles.Permissions").Preload("Roles.Permissions").First(&user, userID).Error; err != nil {
		return UserAccount{}, err
	}
	return user, nil
}

func userIDFromContext(c *gin.Context) (uint, bool) {
	value, exists := c.Get("user")
	if !exists || value == nil {
		return 0, false
	}
	switch user := value.(type) {
	case UserAccount:
		return user.ID, user.ID != 0
	case *UserAccount:
		if user == nil {
			return 0, false
		}
		return user.ID, user.ID != 0
	default:
		return 0, false
	}
}

func buildAccessInfo(user UserAccount) AccessInfo {
	groups := map[string]struct{}{}
	roles := map[string]struct{}{}
	permissions := map[string]struct{}{}

	for _, group := range user.Groups {
		addAccessName(groups, group.Name)
		for _, role := range group.Roles {
			addAccessName(roles, role.Name)
			for _, permission := range role.Permissions {
				addAccessName(permissions, permission.Code)
			}
		}
	}
	for _, role := range user.Roles {
		addAccessName(roles, role.Name)
		for _, permission := range role.Permissions {
			addAccessName(permissions, permission.Code)
		}
	}

	return AccessInfo{
		UserID:      user.ID,
		Username:    user.Username,
		Groups:      sortedAccessNames(groups),
		Roles:       sortedAccessNames(roles),
		Permissions: sortedAccessNames(permissions),
	}
}

func (a *App) visibleMenus(db *gorm.DB, access AccessInfo) ([]AccessMenu, error) {
	var menus []AccessMenu
	if err := db.Where("enabled = ?", true).Order("sort ASC, id ASC").Find(&menus).Error; err != nil {
		return nil, err
	}
	return buildVisibleMenuTree(menus, access, nil), nil
}

func buildVisibleMenuTree(menus []AccessMenu, access AccessInfo, parentID *uint) []AccessMenu {
	var visible []AccessMenu
	for _, menu := range menus {
		if !sameMenuParent(menu.ParentID, parentID) {
			continue
		}
		children := buildVisibleMenuTree(menus, access, &menu.ID)
		canSee := strings.TrimSpace(menu.PermissionCode) == "" || access.HasPermission(menu.PermissionCode)
		if !canSee && len(children) == 0 {
			continue
		}
		menu.Children = children
		visible = append(visible, menu)
	}
	return visible
}

func sameMenuParent(a *uint, b *uint) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func migrateAccessModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&UserAccount{},
		&AccessGroup{},
		&AccessRole{},
		&AccessPermission{},
		&AccessMenu{},
		&RoutePermission{},
		&UserGroup{},
		&UserRole{},
		&GroupRole{},
		&RolePermission{},
	)
}

func containsAny(values []string, expected ...string) bool {
	set := map[string]struct{}{}
	for _, value := range values {
		addAccessName(set, value)
	}
	for _, value := range expected {
		if _, ok := set[normalizeAccessName(value)]; ok {
			return true
		}
	}
	return false
}

func addAccessName(set map[string]struct{}, name string) {
	name = normalizeAccessName(name)
	if name != "" {
		set[name] = struct{}{}
	}
}

func normalizeAccessName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func sortedAccessNames(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}

func ensureBuiltinAdminAccess(db *gorm.DB, account UserAccount) error {
	adminGroup := AccessGroup{Name: "admin", Description: "Built-in administrators"}
	if err := db.Where("name = ?", adminGroup.Name).FirstOrCreate(&adminGroup).Error; err != nil {
		return err
	}

	adminRole := AccessRole{Name: "admin", Description: "Built-in admin role"}
	if err := db.Where("name = ?", adminRole.Name).FirstOrCreate(&adminRole).Error; err != nil {
		return err
	}

	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&UserGroup{UserAccountID: account.ID, AccessGroupID: adminGroup.ID}).Error; err != nil {
		return err
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&GroupRole{AccessGroupID: adminGroup.ID, AccessRoleID: adminRole.ID}).Error; err != nil {
		return err
	}
	return nil
}
