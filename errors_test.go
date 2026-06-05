package sgin

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestHandleErrorDoesNotExposeInternalDetails(t *testing.T) {
	app := newErrorTestApp(t)
	app.GET("/boom", func(c *Context) {
		HandleError(c, fmt.Errorf("database driver failed at C:/secret/app.db"))
	})

	w := performRequest(app, http.MethodGet, "/boom", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	assertResponseMessage(t, w, "Internal Server Error")
	if strings.Contains(w.Body.String(), "database driver") || strings.Contains(w.Body.String(), "C:/secret") {
		t.Fatalf("response leaked internal detail: %s", w.Body.String())
	}
}

func TestHandleErrorUsesConfiguredLanguage(t *testing.T) {
	app := newErrorTestApp(t)
	app.Language("cn")
	app.GET("/bad", func(c *Context) {
		HandleError(c, ErrBadRequest)
	})

	w := performRequest(app, http.MethodGet, "/bad", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	assertResponseMessage(t, w, "请求参数错误")
}

func TestHandleErrorFallsBackToEnglishForUnsupportedLanguage(t *testing.T) {
	app := newErrorTestApp(t)
	app.Language("jp")
	app.GET("/missing", func(c *Context) {
		HandleError(c, ErrNotFound)
	})

	w := performRequest(app, http.MethodGet, "/missing", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	assertResponseMessage(t, w, "Not Found")
}

func TestAppCanRegisterAdditionalLanguage(t *testing.T) {
	app := newErrorTestApp(t)
	app.RegisterLanguage("jp", map[int]string{
		http.StatusBadRequest: "bad request jp",
	})
	app.Language("jp")
	app.GET("/bad", func(c *Context) {
		HandleError(c, ErrBadRequest)
	})

	w := performRequest(app, http.MethodGet, "/bad", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	assertResponseMessage(t, w, "bad request jp")
}

func newErrorTestApp(t *testing.T) *App {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	cfg.Auth.Required = false
	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE: %v", err)
	}
	return app
}
