package sgin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type authViewSetBook struct {
	ID    uint   `json:"id" gorm:"primaryKey"`
	Title string `json:"title"`
}

func TestModelViewSetAuthAllRequiresToken(t *testing.T) {
	app, token := newAuthTestApp(t)
	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath: "/books",
		Auth:     []string{"all"},
	})

	w := performAuthRequest(app, http.MethodGet, "/books", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET without token to be unauthorized, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeAuthenticationRequired)

	w = performAuthRequest(app, http.MethodPost, "/books", "", bytes.NewBufferString(`{"title":"go"}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST without token to be unauthorized, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeAuthenticationRequired)

	w = performAuthRequest(app, http.MethodGet, "/books", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected GET with token to be allowed, got %d", w.Code)
	}
}

func TestModelViewSetAuthSelectedMethods(t *testing.T) {
	app, _ := newAuthTestApp(t)
	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath: "/books",
		Auth:     []string{"get", "delete"},
	})

	w := performAuthRequest(app, http.MethodGet, "/books", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET without token to be unauthorized, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodPost, "/books", "", bytes.NewBufferString(`{"title":"go"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected POST without token to be public, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodDelete, "/books/1", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected DELETE without token to be unauthorized, got %d", w.Code)
	}
}

func TestDefaultAuthRequiresTokenAndAllowsAnonymousAction(t *testing.T) {
	app, token := newGlobalAuthTestApp(t)
	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath:       "/books",
		AllowAnonymous: []string{ActionList},
	})

	w := performAuthRequest(app, http.MethodGet, "/books", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected anonymous list to be allowed, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodPost, "/books", "", bytes.NewBufferString(`{"title":"go"}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected default-protected create to be unauthorized, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeAuthenticationRequired)

	w = performAuthRequest(app, http.MethodPost, "/books", token, bytes.NewBufferString(`{"title":"go"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected authenticated create to be allowed, got %d", w.Code)
	}
}

func TestGlobalAuthCanBeDisabledAndActionCanRequireToken(t *testing.T) {
	app, token := newAuthTestApp(t)
	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath: "/books",
		Auth:     []string{ActionCreate},
	})

	w := performAuthRequest(app, http.MethodGet, "/books", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected default-public list to be allowed, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodPost, "/books", "", bytes.NewBufferString(`{"title":"go"}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected explicitly protected create to be unauthorized, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodPost, "/books", token, bytes.NewBufferString(`{"title":"go"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected authenticated create to be allowed, got %d", w.Code)
	}
}

func TestDefaultAuthDoesNotProtectLoginRoute(t *testing.T) {
	app, _ := newGlobalAuthTestApp(t)

	w := performAuthRequest(app, http.MethodPost, "/login", "", bytes.NewBufferString(`{"username":"missing","password":"missing"}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected login handler to return unauthorized credentials, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeInvalidCredentials)
}

func TestAppAllowAnonymousBypassesDefaultAuthForNativeRoute(t *testing.T) {
	app, token := newGlobalAuthTestApp(t)
	app.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	app.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	app.AllowAnonymous(http.MethodGet, "/health")

	w := performAuthRequest(app, http.MethodGet, "/ping", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected default-protected native route to be unauthorized, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodGet, "/ping", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected authenticated native route to be allowed, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodGet, "/health", "", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected anonymous native route to be allowed, got %d", w.Code)
	}
}

func TestJWTAuthStableErrorCodes(t *testing.T) {
	app, token := newAuthTestApp(t)
	app.GET("/protected", app.JWTAuth(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := performAuthRequest(app, http.MethodGet, "/protected", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing token to be unauthorized, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeAuthenticationRequired)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Token "+token)
	w = httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid authorization header to be unauthorized, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeInvalidAuthorization)

	w = performAuthRequest(app, http.MethodGet, "/protected", "invalid-token", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid token to be unauthorized, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeInvalidToken)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	var user UserAccount
	if err := db.Where("username = ?", "tester").First(&user).Error; err != nil {
		t.Fatalf("expected tester user: %v", err)
	}
	expired, err := makeAuthToken(app.config, &user, "access", -time.Hour)
	if err != nil {
		t.Fatalf("make expired token returned error: %v", err)
	}
	w = performAuthRequest(app, http.MethodGet, "/protected", expired, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected expired token to be unauthorized, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeTokenExpired)

	refresh, err := makeAuthToken(app.config, &user, "refresh", time.Hour)
	if err != nil {
		t.Fatalf("make refresh token returned error: %v", err)
	}
	w = performAuthRequest(app, http.MethodGet, "/protected", refresh, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong token type to be unauthorized, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeInvalidTokenType)

	if err := db.Model(&user).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable user returned error: %v", err)
	}
	w = performAuthRequest(app, http.MethodGet, "/protected", token, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled account to be unauthorized, got %d", w.Code)
	}
	assertResponseError(t, w, ErrCodeAccountDisabled)
}

func TestModelViewSetAuthRejectsUnknownMethod(t *testing.T) {
	app, _ := newAuthTestApp(t)
	defer func() {
		if recover() == nil {
			t.Fatal("expected invalid Auth method to panic")
		}
	}()

	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath: "/books",
		Auth:     []string{"gett"},
	})
}

func TestModelViewSetMiddlewaresAndHandlers(t *testing.T) {
	app, _ := newAuthTestApp(t)
	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath: "/books",
		Middlewares: []gin.HandlerFunc{
			func(c *gin.Context) {
				c.Set("viewset", "ok")
				c.Next()
			},
		},
		ActionMiddlewares: map[string][]gin.HandlerFunc{
			"post": {
				func(c *gin.Context) {
					c.Set("action", "post")
					c.Next()
				},
			},
		},
		Handlers: map[string]gin.HandlerFunc{
			"post": func(c *gin.Context) {
				viewset, _ := c.Get("viewset")
				action, _ := c.Get("action")
				if viewset != "ok" || action != "post" {
					c.Status(http.StatusInternalServerError)
					return
				}
				c.Status(http.StatusAccepted)
			},
		},
	})

	w := performAuthRequest(app, http.MethodPost, "/books", "", bytes.NewBufferString(`{"title":"go"}`))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected POST to use custom handler, got %d", w.Code)
	}
}

func TestModelViewSetExtraActionsUseAuthAndMiddlewares(t *testing.T) {
	app, token := newAuthTestApp(t)
	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath: "/books",
		Auth:     []string{"all"},
		Middlewares: []gin.HandlerFunc{
			func(c *gin.Context) {
				c.Set("viewset", "ok")
				c.Next()
			},
		},
		ExtraActions: []ExtraAction{
			{
				Method: "post",
				Path:   "reset-password",
				Detail: true,
				Middlewares: []gin.HandlerFunc{
					func(c *gin.Context) {
						c.Set("extra", "ok")
						c.Next()
					},
				},
				Handler: func(c *gin.Context) {
					viewset, _ := c.Get("viewset")
					extra, _ := c.Get("extra")
					if c.Param("id") != "1" || viewset != "ok" || extra != "ok" {
						c.Status(http.StatusInternalServerError)
						return
					}
					c.Status(http.StatusAccepted)
				},
			},
			{
				Method: "post",
				Path:   "export",
				Handler: func(c *gin.Context) {
					c.Status(http.StatusOK)
				},
			},
			{
				Method: "get",
				Path:   "hot",
				Middlewares: []gin.HandlerFunc{
					func(c *gin.Context) {
						c.Set("extra", "hot")
						c.Next()
					},
				},
				Handler: func(c *gin.Context) {
					viewset, _ := c.Get("viewset")
					extra, _ := c.Get("extra")
					if viewset != "ok" || extra != "hot" {
						c.Status(http.StatusInternalServerError)
						return
					}
					c.Status(http.StatusOK)
				},
			},
		},
	})

	w := performAuthRequest(app, http.MethodPost, "/books/1/reset-password", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected extra detail action without token to be unauthorized, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodPost, "/books/1/reset-password", token, nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected extra detail action with token to be allowed, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodPost, "/books/export", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected extra collection action with token to be allowed, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodGet, "/books/hot", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected dispatched collection action without token to be unauthorized, got %d", w.Code)
	}

	w = performAuthRequest(app, http.MethodGet, "/books/hot", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected dispatched collection action with token to be allowed, got %d", w.Code)
	}
}

func TestModelViewSetRejectsUnknownRouteExtensionMethod(t *testing.T) {
	app, _ := newAuthTestApp(t)
	defer func() {
		if recover() == nil {
			t.Fatal("expected invalid route extension method to panic")
		}
	}()

	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath: "/books",
		ActionMiddlewares: map[string][]gin.HandlerFunc{
			"remove": nil,
		},
	})
}

func TestModelViewSetRejectsInvalidExtraAction(t *testing.T) {
	app, _ := newAuthTestApp(t)
	defer func() {
		if recover() == nil {
			t.Fatal("expected invalid extra action to panic")
		}
	}()

	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath: "/books",
		ExtraActions: []ExtraAction{
			{
				Method:  "post",
				Path:    "",
				Handler: func(c *gin.Context) {},
			},
		},
	})
}

func TestModelViewSetRejectsCollectionExtraActionThatLooksLikeID(t *testing.T) {
	app, _ := newAuthTestApp(t)
	defer func() {
		if recover() == nil {
			t.Fatal("expected ID-like collection extra action to panic")
		}
	}()

	app.Register(&ModelViewSet[authViewSetBook, uint]{
		BasePath: "/books",
		ExtraActions: []ExtraAction{
			{
				Method:  "get",
				Path:    "123",
				Handler: func(c *gin.Context) {},
			},
		},
	})
}
func newAuthTestApp(t *testing.T) (*App, string) {
	return newAuthTestAppWithDefaultAuth(t, false)
}

func newGlobalAuthTestApp(t *testing.T) (*App, string) {
	return newAuthTestAppWithDefaultAuth(t, true)
}

func newAuthTestAppWithDefaultAuth(t *testing.T, required bool) (*App, string) {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "auth.db")
	cfg.Database.AutoMigrate = true
	cfg.User.Enabled = true
	cfg.User.Admin.Init = false
	cfg.Auth.Required = required
	cfg.JWT.Secret = "auth-test-secret"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	if err := db.AutoMigrate(&UserAccount{}, &authViewSetBook{}); err != nil {
		t.Fatalf("AutoMigrate returned error: %v", err)
	}

	user := UserAccount{Username: "tester", PasswordHash: "hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user returned error: %v", err)
	}
	token, err := makeAuthToken(cfg, &user, "access", time.Hour)
	if err != nil {
		t.Fatalf("makeAuthToken returned error: %v", err)
	}
	return app, token
}

func performAuthRequest(app *App, method, path, token string, body *bytes.Buffer) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body == nil {
		reqBody = bytes.NewBuffer(nil)
	} else {
		reqBody = body
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	return w
}

// assertResponseError 检查统一响应里的稳定错误码。
func assertResponseError(t *testing.T, w *httptest.ResponseRecorder, code string) {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Error != code {
		t.Fatalf("expected error code %q, got %v body=%s", code, resp.Error, w.Body.String())
	}
}
