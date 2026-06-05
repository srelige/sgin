package sgin

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type apiViewCar struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Name string `json:"name"`
}

func TestAPIViewGetListUsesDefaultRepository(t *testing.T) {
	app := newAPIViewTestApp(t)
	seedAPIViewCars(t, app)

	app.Register(&APIView[apiViewCar, uint]{
		Method: "get",
		Path:   "/cars/",
	})

	w := performAPIViewRequest(app, http.MethodGet, "/cars", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected GET /cars to return 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "sedan") || !strings.Contains(w.Body.String(), "suv") {
		t.Fatalf("expected response to contain seeded cars, got %s", w.Body.String())
	}
}

func TestAPIViewGetRetrieveUsesDefaultRepository(t *testing.T) {
	app := newAPIViewTestApp(t)
	seedAPIViewCars(t, app)

	app.Register(&APIView[apiViewCar, uint]{
		Method: "get",
		Path:   "/cars/:id",
	})

	w := performAPIViewRequest(app, http.MethodGet, "/cars/1", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected GET /cars/1 to return 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "sedan") {
		t.Fatalf("expected response to contain selected car, got %s", w.Body.String())
	}
}

func TestAPIViewAuthAndMiddleware(t *testing.T) {
	app, token := newAuthTestApp(t)
	app.Register(&APIView[authViewSetBook, uint]{
		Method: "get",
		Path:   "/books",
		Auth:   []string{"all"},
		Middlewares: []gin.HandlerFunc{
			func(c *gin.Context) {
				c.Set("route", "api")
				c.Next()
			},
		},
		Handler: func(c *gin.Context) {
			value, _ := c.Get("route")
			if value != "api" {
				c.Status(http.StatusInternalServerError)
				return
			}
			c.Status(http.StatusAccepted)
		},
	})

	w := performAPIViewRequest(app, http.MethodGet, "/books", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated request to be rejected, got %d", w.Code)
	}

	w = performAPIViewRequest(app, http.MethodGet, "/books", token, "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected authenticated request to use custom handler, got %d", w.Code)
	}
}

func TestAPIViewRejectsInvalidDefaultPath(t *testing.T) {
	app := newAPIViewTestApp(t)
	defer func() {
		if recover() == nil {
			t.Fatal("expected invalid APIView default path to panic")
		}
	}()

	app.Register(&APIView[apiViewCar, uint]{
		Method: "delete",
		Path:   "/cars",
	})
}

func newAPIViewTestApp(t *testing.T) *App {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "api-view.db")
	cfg.Database.AutoMigrate = true
	cfg.User.Enabled = false
	cfg.Auth.Required = false
	cfg.JWT.Secret = "api-view-secret"

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	t.Cleanup(app.CloseDB)

	if err := app.InitTable(&apiViewCar{}); err != nil {
		t.Fatalf("InitTable returned error: %v", err)
	}
	return app
}

func seedAPIViewCars(t *testing.T, app *App) {
	t.Helper()
	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB returned error: %v", err)
	}
	if err := db.Create(&apiViewCar{Name: "sedan"}).Error; err != nil {
		t.Fatalf("create sedan returned error: %v", err)
	}
	if err := db.Create(&apiViewCar{Name: "suv"}).Error; err != nil {
		t.Fatalf("create suv returned error: %v", err)
	}
}

func performAPIViewRequest(app *App, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	return w
}
