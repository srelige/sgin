package sgin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestLoginResponseIncludesAccessSnapshotMenusAndPermissions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "login-menus.db")
	cfg.User.Enabled = true
	cfg.User.Admin.Init = false
	cfg.JWT.Secret = "login-menus-secret"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password returned error: %v", err)
	}
	user := UserAccount{Username: "finance", PasswordHash: string(hash), Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user returned error: %v", err)
	}
	group := AccessGroup{Name: "finance"}
	role := AccessRole{Name: "finance_reader"}
	permission := AccessPermission{Code: "finance.menu", Name: "Finance menu"}
	hiddenPermission := AccessPermission{Code: "ops.menu", Name: "Ops menu"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group returned error: %v", err)
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role returned error: %v", err)
	}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatalf("create permission returned error: %v", err)
	}
	if err := db.Create(&hiddenPermission).Error; err != nil {
		t.Fatalf("create hidden permission returned error: %v", err)
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
	if err := db.Create(&AccessMenu{Name: "root", Title: "Root", Sort: 1, Enabled: true}).Error; err != nil {
		t.Fatalf("create root menu returned error: %v", err)
	}
	var root AccessMenu
	if err := db.Where("name = ?", "root").First(&root).Error; err != nil {
		t.Fatalf("query root menu returned error: %v", err)
	}
	if err := db.Create(&AccessMenu{Name: "finance", Title: "Finance", Path: "/finance", PermissionCode: "finance.menu", ParentID: &root.ID, Sort: 2, Enabled: true}).Error; err != nil {
		t.Fatalf("create finance menu returned error: %v", err)
	}
	if err := db.Create(&AccessMenu{Name: "ops", Title: "Ops", Path: "/ops", PermissionCode: "ops.menu", Sort: 3, Enabled: true}).Error; err != nil {
		t.Fatalf("create ops menu returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"username":"finance","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response returned error: %v", err)
	}
	tokens := responseData[tokenResponse](t, resp)
	if tokens.User == nil || tokens.User.Username != "finance" {
		t.Fatalf("expected login user snapshot, got %+v", tokens.User)
	}
	if len(tokens.Permissions) != 1 || tokens.Permissions[0] != "finance.menu" {
		t.Fatalf("expected permission snapshot, got %+v", tokens.Permissions)
	}
	if len(tokens.Menus) != 1 || tokens.Menus[0].Name != "root" || len(tokens.Menus[0].Children) != 1 || tokens.Menus[0].Children[0].Name != "finance" {
		t.Fatalf("expected filtered menu tree, got %+v", tokens.Menus)
	}
}
