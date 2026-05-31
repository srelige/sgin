package sgin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestAdminEnabledCreatesAdminAccount(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = true
	cfg.User.Admin.Init = true
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(dir, "app.db")

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	result := app.AdminBootstrapResult()
	if !result.Created {
		t.Fatal("expected admin account to be created")
	}
	if result.Username != "admin" {
		t.Fatalf("expected admin username, got %q", result.Username)
	}
	if result.Password == "" {
		t.Fatal("expected generated plaintext password")
	}

	db, err := gorm.Open(sqlite.Open(cfg.Database.DSN), userGormConfig())
	if err != nil {
		t.Fatalf("open sqlite with gorm: %v", err)
	}
	defer closeGormDB(db)

	var account AdminAccount
	if err := db.Where("username = ?", "admin").First(&account).Error; err != nil {
		t.Fatalf("query admin account: %v", err)
	}
	if account.PasswordHash == result.Password {
		t.Fatal("password should be stored as a hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(result.Password)); err != nil {
		t.Fatalf("stored hash does not match generated password: %v", err)
	}
	if !account.Admin {
		t.Fatal("expected admin flag true")
	}
}

func TestAdminEnabledIgnoresExistingAdminAccount(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = true
	cfg.User.Admin.Init = true
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(dir, "app.db")

	first, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("first NewE returned error: %v", err)
	}
	if !first.AdminBootstrapResult().Created {
		t.Fatal("expected first startup to create admin account")
	}

	second, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("second NewE returned error: %v", err)
	}
	result := second.AdminBootstrapResult()
	if result.Created {
		t.Fatal("expected existing admin account to be ignored")
	}
	if result.Password != "" {
		t.Fatal("expected no plaintext password when admin already exists")
	}
}

func TestExistingAdminAccountGetsBuiltinAccess(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = true
	cfg.User.Admin.Init = true
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(dir, "app.db")

	db, err := gorm.Open(sqlite.Open(cfg.Database.DSN), userGormConfig())
	if err != nil {
		t.Fatalf("open sqlite with gorm: %v", err)
	}
	if err := migrateAccessModels(db); err != nil {
		t.Fatalf("migrate access models: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("existing-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	account := UserAccount{Username: "admin", PasswordHash: string(hash), Enabled: true}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create existing admin account: %v", err)
	}
	closeGormDB(db)

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	result := app.AdminBootstrapResult()
	if result.Created {
		t.Fatal("expected existing admin account to be reused")
	}
	if result.Password != "" {
		t.Fatal("expected no plaintext password for existing admin")
	}

	db, err = app.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	var group AccessGroup
	if err := db.Where("name = ?", "admin").First(&group).Error; err != nil {
		t.Fatalf("expected admin group: %v", err)
	}
	var membership UserGroup
	if err := db.Where("user_account_id = ? AND access_group_id = ?", account.ID, group.ID).First(&membership).Error; err != nil {
		t.Fatalf("expected existing admin group membership: %v", err)
	}
}

func TestUserLoginAndRefreshToken(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = true
	cfg.User.Admin.Init = true
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(dir, "app.db")

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)
	admin := app.AdminBootstrapResult()
	if !admin.Created {
		t.Fatal("expected admin account to be created")
	}

	app.GET("/me", app.JWTAuth(), func(c *Context) {
		user, exists := c.Get("user")
		if !exists {
			t.Fatal("expected user in context")
		}
		account := user.(UserAccount)
		c.JSON(http.StatusOK, H{"username": account.Username, "admin": account.Admin})
	})

	loginBody := []byte(`{"username":"admin","password":"` + admin.Password + `"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d body=%s", w.Code, w.Body.String())
	}

	var loginResp Response
	if err := json.Unmarshal(w.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	tokens := responseData[tokenResponse](t, loginResp)
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("expected access and refresh tokens")
	}
	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	var account UserAccount
	if err := db.Where("username = ?", "admin").First(&account).Error; err != nil {
		t.Fatalf("query admin account: %v", err)
	}
	if account.RefreshTokenHash == "" || account.RefreshTokenHash == tokens.RefreshToken {
		t.Fatal("expected refresh token to be stored as a hash")
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected protected route 200, got %d body=%s", w.Code, w.Body.String())
	}

	refreshBody := []byte(`{"refresh_token":"` + tokens.RefreshToken + `"}`)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/login/refresh", bytes.NewReader(refreshBody))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected refresh 200, got %d body=%s", w.Code, w.Body.String())
	}
	var refreshResp Response
	if err := json.Unmarshal(w.Body.Bytes(), &refreshResp); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	rotatedTokens := responseData[tokenResponse](t, refreshResp)
	if rotatedTokens.RefreshToken == "" || rotatedTokens.RefreshToken == tokens.RefreshToken {
		t.Fatal("expected refresh token to rotate")
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/login/refresh", bytes.NewReader(refreshBody))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected old refresh token to be rejected, got %d body=%s", w.Code, w.Body.String())
	}
}

func responseData[T any](t *testing.T, resp Response) T {
	t.Helper()
	data, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal response data: %v", err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	return out
}
