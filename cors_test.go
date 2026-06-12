package sgin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSMiddlewareHandlesPreflightBeforeAuth(t *testing.T) {
	app := newCORSTestApp(t, true)

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Authorization")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status 204, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5173" {
		t.Fatalf("expected allow origin header, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("expected credentials header")
	}
	if rec.Header().Get("Access-Control-Max-Age") != "1800" {
		t.Fatalf("expected max age header, got %q", rec.Header().Get("Access-Control-Max-Age"))
	}
}

func TestCORSMiddlewareAddsHeadersForActualRequest(t *testing.T) {
	app := newCORSTestApp(t, false)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5173" {
		t.Fatalf("expected allow origin header, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Expose-Headers") != "X-Trace-ID" {
		t.Fatalf("expected expose headers, got %q", rec.Header().Get("Access-Control-Expose-Headers"))
	}
}

func TestCORSMiddlewareDefaultDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	cfg.Auth.Required = false

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	app.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("expected no CORS header when disabled, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func newCORSTestApp(t *testing.T, authRequired bool) *App {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	cfg.Auth.Required = authRequired
	cfg.CORS.Enabled = true
	cfg.CORS.AllowOrigins = []string{"http://127.0.0.1:5173"}
	cfg.CORS.AllowMethods = []string{"GET", "POST", "OPTIONS"}
	cfg.CORS.AllowHeaders = []string{"Authorization", "Content-Type"}
	cfg.CORS.ExposeHeaders = []string{"X-Trace-ID"}
	cfg.CORS.AllowCredentials = true
	cfg.CORS.MaxAge = "30m"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	app.GET("/ping", func(c *gin.Context) {
		c.Header("X-Trace-ID", "trace-1")
		c.String(http.StatusOK, "pong")
	})
	return app
}
