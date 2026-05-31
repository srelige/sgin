package sgin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAdminUIAPIRequiresAdminRole(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "admin-ui.db")
	cfg.User.Enabled = true
	cfg.User.Admin.Init = false
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/sgin-admin"
	cfg.JWT.Secret = "admin-ui-secret"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	user := UserAccount{Username: "admin-ui", PasswordHash: "hash", Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user returned error: %v", err)
	}
	role := AccessRole{Name: "admin"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create admin 组 returned error: %v", err)
	}
	group := AccessGroup{Name: "admin"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create admin group returned error: %v", err)
	}
	if err := db.Create(&GroupRole{AccessGroupID: group.ID, AccessRoleID: role.ID}).Error; err != nil {
		t.Fatalf("create group role returned error: %v", err)
	}
	if err := db.Create(&UserGroup{UserAccountID: user.ID, AccessGroupID: group.ID}).Error; err != nil {
		t.Fatalf("create user group returned error: %v", err)
	}
	token, err := makeAuthToken(cfg, &user, "access", time.Hour)
	if err != nil {
		t.Fatalf("makeAuthToken returned error: %v", err)
	}

	w := performAdminUIRequest(app, "/sgin-admin/api/state", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected admin API without token to be unauthorized, got %d", w.Code)
	}

	w = performAdminUIRequest(app, "/sgin-admin/api/state", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected admin API with admin token to be allowed, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "route_permissions") {
		t.Fatalf("expected admin state response, got %s", w.Body.String())
	}
}

func TestAdminUIPageServedFromEmbeddedAsset(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "admin-ui-page.db")
	cfg.User.Enabled = true
	cfg.User.Admin.Init = false
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/sgin-admin"
	cfg.JWT.Secret = "admin-ui-page-secret"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sgin-admin", nil)
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected admin UI page to return 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "sgin admin") {
		t.Fatalf("expected embedded admin UI page, got %s", w.Body.String())
	}
}

func TestAdminUIRejectsInvalidRoutePermissionMethod(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "admin-ui-invalid-method.db")
	cfg.User.Enabled = true
	cfg.User.Admin.Init = false
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/sgin-admin"
	cfg.JWT.Secret = "admin-ui-invalid-method-secret"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	user := UserAccount{Username: "admin-ui", PasswordHash: "hash", Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user returned error: %v", err)
	}
	role := AccessRole{Name: "admin"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create admin 组 returned error: %v", err)
	}
	group := AccessGroup{Name: "admin"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create admin group returned error: %v", err)
	}
	if err := db.Create(&GroupRole{AccessGroupID: group.ID, AccessRoleID: role.ID}).Error; err != nil {
		t.Fatalf("create group role returned error: %v", err)
	}
	if err := db.Create(&UserGroup{UserAccountID: user.ID, AccessGroupID: group.ID}).Error; err != nil {
		t.Fatalf("create user group returned error: %v", err)
	}
	token, err := makeAuthToken(cfg, &user, "access", time.Hour)
	if err != nil {
		t.Fatalf("makeAuthToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/sgin-admin/api/route-permissions", strings.NewReader(`{"method":"GGT","path":"/cmdb","permission_code":"cmdb.view"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid method to return 400, got %d", w.Code)
	}
}

func TestAdminUIAPIManagesUsersAndAccessRecords(t *testing.T) {
	app, token := newAdminUITestApp(t, "admin-ui-workflow.db")

	user := adminUIJSON[UserAccount](t, app, token, http.MethodPost, "/sgin-admin/api/users", `{"username":"member","password":"secret","enabled":false}`, http.StatusCreated)
	if user.ID == 0 || user.Username != "member" || user.Enabled {
		t.Fatalf("unexpected created user: %+v", user)
	}

	adminUIJSON[map[string]bool](t, app, token, http.MethodPatch, "/sgin-admin/api/users/"+uintText(user.ID)+"/enabled", `{"enabled":true}`, http.StatusOK)
	group := adminUIJSON[AccessGroup](t, app, token, http.MethodPost, "/sgin-admin/api/groups", `{"name":"member","description":"members"}`, http.StatusCreated)
	role := adminUIJSON[AccessRole](t, app, token, http.MethodPost, "/sgin-admin/api/roles", `{"name":"member_reader","description":"reader"}`, http.StatusCreated)
	permission := adminUIJSON[AccessPermission](t, app, token, http.MethodPost, "/sgin-admin/api/permissions", `{"code":"member.view","name":"View member"}`, http.StatusCreated)
	route := adminUIJSON[RoutePermission](t, app, token, http.MethodPost, "/sgin-admin/api/route-permissions", `{"method":"get","path":"/members","permission_code":"member.view","enabled":true}`, http.StatusCreated)

	if group.ID == 0 || role.ID == 0 || permission.ID == 0 || route.ID == 0 {
		t.Fatal("expected created access records to have ids")
	}
	if route.Method != http.MethodGet || route.Path != "/members" || route.PermissionCode != "member.view" || !route.Enabled {
		t.Fatalf("unexpected route permission: %+v", route)
	}

	adminUIJSON[UserGroup](t, app, token, http.MethodPost, "/sgin-admin/api/users/"+uintText(user.ID)+"/groups", `{"group_id":`+uintText(group.ID)+`}`, http.StatusCreated)
	adminUIJSON[UserRole](t, app, token, http.MethodPost, "/sgin-admin/api/users/"+uintText(user.ID)+"/roles", `{"role_id":`+uintText(role.ID)+`}`, http.StatusCreated)
	adminUIJSON[GroupRole](t, app, token, http.MethodPost, "/sgin-admin/api/groups/"+uintText(group.ID)+"/roles", `{"role_id":`+uintText(role.ID)+`}`, http.StatusCreated)
	adminUIJSON[RolePermission](t, app, token, http.MethodPost, "/sgin-admin/api/roles/"+uintText(role.ID)+"/permissions", `{"permission_id":`+uintText(permission.ID)+`}`, http.StatusCreated)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	var saved UserAccount
	if err := db.Where("username = ?", "member").First(&saved).Error; err != nil {
		t.Fatalf("query created user returned error: %v", err)
	}
	if !saved.Enabled {
		t.Fatal("expected user enabled flag to be updated")
	}
	if saved.PasswordHash == "" || saved.PasswordHash == "secret" {
		t.Fatal("expected admin-created password to be hashed")
	}
	var userGroup UserGroup
	if err := db.Where("user_account_id = ? AND access_group_id = ?", saved.ID, group.ID).First(&userGroup).Error; err != nil {
		t.Fatalf("expected user group binding: %v", err)
	}
	var rolePermission RolePermission
	if err := db.Where("access_role_id = ? AND access_permission_id = ?", role.ID, permission.ID).First(&rolePermission).Error; err != nil {
		t.Fatalf("expected role permission binding: %v", err)
	}
}

func performAdminUIRequest(app *App, path string, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	return w
}

func newAdminUITestApp(t *testing.T, dbName string) (*App, string) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), dbName)
	cfg.User.Enabled = true
	cfg.User.Admin.Init = false
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/sgin-admin"
	cfg.JWT.Secret = "admin-ui-helper-secret"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	user := UserAccount{Username: "admin-ui", PasswordHash: "hash", Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user returned error: %v", err)
	}
	group := AccessGroup{Name: "admin"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create admin group returned error: %v", err)
	}
	if err := db.Create(&UserGroup{UserAccountID: user.ID, AccessGroupID: group.ID}).Error; err != nil {
		t.Fatalf("create user group returned error: %v", err)
	}
	token, err := makeAuthToken(cfg, &user, "access", time.Hour)
	if err != nil {
		t.Fatalf("makeAuthToken returned error: %v", err)
	}
	return app, token
}

func adminUIJSON[T any](t *testing.T, app *App, token, method, path, body string, expectedStatus int) T {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != expectedStatus {
		t.Fatalf("expected %s %s to return %d, got %d body=%s", method, path, expectedStatus, w.Code, w.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode admin response: %v", err)
	}
	return responseData[T](t, resp)
}

func uintText(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
