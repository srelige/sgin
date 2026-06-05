package sgin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func TestAdminBootstrapCreatesAdminAccess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "admin-access.db")
	cfg.User.Enabled = true
	cfg.User.Admin.Init = true
	cfg.User.Admin.Username = "admin"
	cfg.User.Admin.Password = "admin-access-password"
	cfg.JWT.Secret = "admin-access-secret"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}

	var user UserAccount
	if err := db.Where("username = ?", "admin").First(&user).Error; err != nil {
		t.Fatalf("expected admin user: %v", err)
	}
	var group AccessGroup
	if err := db.Where("name = ?", "admin").First(&group).Error; err != nil {
		t.Fatalf("expected admin group: %v", err)
	}
	var role AccessRole
	if err := db.Where("name = ?", "admin").First(&role).Error; err != nil {
		t.Fatalf("expected admin 组: %v", err)
	}

	var userGroup UserGroup
	if err := db.Where("user_account_id = ? AND access_group_id = ?", user.ID, group.ID).First(&userGroup).Error; err != nil {
		t.Fatalf("expected admin user group membership: %v", err)
	}
	var groupRole GroupRole
	if err := db.Where("access_group_id = ? AND access_role_id = ?", group.ID, role.ID).First(&groupRole).Error; err != nil {
		t.Fatalf("expected admin group role membership: %v", err)
	}
}

func TestDisabledUserLoginReturnsDisabledBeforePassword(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "disabled-login.db")
	cfg.User.Enabled = true
	cfg.User.Admin.Init = false
	cfg.JWT.Secret = "disabled-login-secret"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("right-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password returned error: %v", err)
	}
	disabled := UserAccount{Username: "disabled", PasswordHash: string(hash), Enabled: true}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatalf("create disabled user returned error: %v", err)
	}
	if err := db.Model(&disabled).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable user returned error: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, cfg.User.Path, bytes.NewBufferString(`{"username":"disabled","password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled login to be unauthorized, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Unauthorized") {
		t.Fatalf("expected disabled login response, got %s", w.Body.String())
	}
}

func TestAccessMiddlewaresRequireGroupRoleAndRoutePermission(t *testing.T) {
	app, token := newAuthTestApp(t)
	grantTesterAccess(t, app)

	app.GET("/cmdb-group", app.JWTAuth(), app.LoadAccess(), app.RequireAnyGroup("dev", "ops"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	app.GET("/cmdb-role", app.JWTAuth(), app.LoadAccess(), app.RequireAnyRole("cmdb_reader"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	app.GET("/cmdb-route", app.JWTAuth(), app.LoadAccess(), app.RequireRoutePermission(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for _, path := range []string{"/cmdb-group", "/cmdb-role", "/cmdb-route"} {
		w := accessRequest(app, path, token)
		if w.Code != http.StatusOK {
			t.Fatalf("expected %s to be allowed, got %d", path, w.Code)
		}
	}
}

func TestModelViewSetExtraActionUsesRoutePermissionPath(t *testing.T) {
	app, token := newAuthTestApp(t)
	grantTesterAccess(t, app)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	if err := db.Create(&RoutePermission{Method: http.MethodPost, Path: "/assets/:id/sync", PermissionCode: "cmdb.view", Enabled: true}).Error; err != nil {
		t.Fatalf("create extra action route permission returned error: %v", err)
	}
	if err := db.Create(&RoutePermission{Method: http.MethodGet, Path: "/assets/export", PermissionCode: "cmdb.view", Enabled: true}).Error; err != nil {
		t.Fatalf("create collection extra action route permission returned error: %v", err)
	}

	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath:   "/assets",
		Serializer: FullModelSerializer[authViewSetBook]{},
		Auth:       []string{"all"},
		Middlewares: []gin.HandlerFunc{
			app.LoadAccess(),
			app.RequireRoutePermission(),
		},
		ExtraActions: []ExtraAction{
			{
				Method: "post",
				Path:   "sync",
				Detail: true,
				Handler: func(c *gin.Context) {
					c.Status(http.StatusOK)
				},
			},
			{
				Method: "get",
				Path:   "export",
				Handler: func(c *gin.Context) {
					c.Status(http.StatusOK)
				},
			},
		},
	})

	w := performAuthRequest(app, http.MethodPost, "/assets/1/sync", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected extra action route permission to allow request, got %d", w.Code)
	}

	w = accessRequest(app, "/assets/export", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected dispatched extra action route permission to allow request, got %d", w.Code)
	}
}

func TestAccessMiddlewaresDenyMissingGroupRoleAndPermission(t *testing.T) {
	app, token := newAuthTestApp(t)
	grantTesterAccess(t, app)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	if err := db.Create(&RoutePermission{Method: http.MethodGet, Path: "/cmdb-route-denied", PermissionCode: "cmdb.delete", Enabled: true}).Error; err != nil {
		t.Fatalf("create denied route permission returned error: %v", err)
	}

	app.GET("/cmdb-group-denied", app.JWTAuth(), app.LoadAccess(), app.RequireAnyGroup("qa"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	app.GET("/cmdb-role-denied", app.JWTAuth(), app.LoadAccess(), app.RequireAnyRole("cmdb_writer"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	app.GET("/cmdb-route-missing", app.JWTAuth(), app.LoadAccess(), app.RequireRoutePermission(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	app.GET("/cmdb-route-denied", app.JWTAuth(), app.LoadAccess(), app.RequireRoutePermission(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	cases := []struct {
		path string
	}{
		{path: "/cmdb-group-denied"},
		{path: "/cmdb-role-denied"},
		{path: "/cmdb-route-missing"},
		{path: "/cmdb-route-denied"},
	}
	for _, tc := range cases {
		w := accessRequest(app, tc.path, token)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected %s to be forbidden, got %d", tc.path, w.Code)
		}
		assertResponseMessage(t, w, "Forbidden")
	}
}

func TestAdminGroupBypassesRoutePermission(t *testing.T) {
	app, token := newAuthTestApp(t)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	var user UserAccount
	if err := db.Where("username = ?", "tester").First(&user).Error; err != nil {
		t.Fatalf("expected tester user: %v", err)
	}
	group := AccessGroup{Name: "admin"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create admin group returned error: %v", err)
	}
	if err := db.Create(&UserGroup{UserAccountID: user.ID, AccessGroupID: group.ID}).Error; err != nil {
		t.Fatalf("create admin user group returned error: %v", err)
	}

	app.GET("/admin-bypass", app.JWTAuth(), app.LoadAccess(), app.RequireRoutePermission(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := accessRequest(app, "/admin-bypass", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected admin group to bypass route permission, got %d", w.Code)
	}
}

func grantTesterAccess(t *testing.T, app *App) {
	t.Helper()
	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}

	var user UserAccount
	if err := db.Where("username = ?", "tester").First(&user).Error; err != nil {
		t.Fatalf("expected tester user: %v", err)
	}
	group := AccessGroup{Name: "dev"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group returned error: %v", err)
	}
	role := AccessRole{Name: "cmdb_reader"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role returned error: %v", err)
	}
	permission := AccessPermission{Code: "cmdb.view", Name: "View CMDB"}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatalf("create permission returned error: %v", err)
	}
	if err := db.Create(&UserGroup{UserAccountID: user.ID, AccessGroupID: group.ID}).Error; err != nil {
		t.Fatalf("create user group returned error: %v", err)
	}
	if err := db.Create(&GroupRole{AccessGroupID: group.ID, AccessRoleID: role.ID}).Error; err != nil {
		t.Fatalf("create group role returned error: %v", err)
	}
	if err := db.Create(&RolePermission{AccessRoleID: role.ID, AccessPermissionID: permission.ID}).Error; err != nil {
		t.Fatalf("create role permission returned error: %v", err)
	}
	if err := db.Create(&RoutePermission{Method: http.MethodGet, Path: "/cmdb-route", PermissionCode: permission.Code, Enabled: true}).Error; err != nil {
		t.Fatalf("create route permission returned error: %v", err)
	}
}

func accessRequest(app *App, path string, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	return w
}
