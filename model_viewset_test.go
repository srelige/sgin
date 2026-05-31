package sgin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type testUser struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type gormViewSetUser struct {
	ID     uint   `json:"id" gorm:"primaryKey"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Status string `json:"status"`
}

type gormFilterBook struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Name string `json:"name"`
	Info string `json:"info"`
	Year int    `json:"year"`
}

type memoryUserRepo struct {
	items     map[uint]testUser
	lastQuery Query
}

type denyTestObjectPermission struct{}

func (denyTestObjectPermission) HasObjectPermission(c *Context, action string, obj *testUser) Decision {
	return Deny("object_denied", "object denied")
}

type statusScopePermission struct{}

func (statusScopePermission) ScopeQuery(c *Context, action string, q Query) Query {
	if q.Filters == nil {
		q.Filters = map[string]string{}
	}
	q.Filters["status"] = "active"
	return q
}

func newMemoryUserRepo() *memoryUserRepo {
	return &memoryUserRepo{
		items: map[uint]testUser{
			1: {ID: 1, Name: "Tom", Email: "tom@example.com"},
			2: {ID: 2, Name: "Jerry", Email: "jerry@example.com"},
		},
	}
}

func (r *memoryUserRepo) List(ctx context.Context, q Query) ([]testUser, error) {
	r.lastQuery = q
	keys := make([]int, 0, len(r.items))
	for id := range r.items {
		keys = append(keys, int(id))
	}
	sort.Ints(keys)

	out := make([]testUser, 0, len(keys))
	for _, id := range keys {
		out = append(out, r.items[uint(id)])
	}
	return out, nil
}

func (r *memoryUserRepo) Find(ctx context.Context, id uint) (*testUser, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &item, nil
}

func (r *memoryUserRepo) Create(ctx context.Context, obj *testUser) error {
	if obj.ID == 0 {
		obj.ID = uint(len(r.items) + 1)
	}
	r.items[obj.ID] = *obj
	return nil
}

func (r *memoryUserRepo) Update(ctx context.Context, obj *testUser) error {
	r.items[obj.ID] = *obj
	return nil
}

func (r *memoryUserRepo) Delete(ctx context.Context, id uint) error {
	delete(r.items, id)
	return nil
}

func TestModelViewSetCRUDAndQueryParsing(t *testing.T) {
	app := newTestApp(t)
	app.config.REST.Pagination = true
	repo := newMemoryUserRepo()
	app.Register(&ModelViewSet[testUser, uint]{
		BasePath:   "/users",
		Repository: repo,
	})

	w := performRequest(app, http.MethodGet, "/users?page=3&page_size=1&search=tom&ordering=-id&status=active", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if repo.lastQuery.Limit != 1 || repo.lastQuery.Offset != 2 {
		t.Fatalf("unexpected pagination query: %+v", repo.lastQuery)
	}
	if repo.lastQuery.Search != "tom" || repo.lastQuery.OrderBy != "-id" {
		t.Fatalf("unexpected search/order query: %+v", repo.lastQuery)
	}
	if repo.lastQuery.Filters["status"] != "active" {
		t.Fatalf("expected field filter status=active, got %+v", repo.lastQuery.Filters)
	}

	body := bytes.NewBufferString(`{"name":"Alice","email":"alice@example.com"}`)
	w = performRequest(app, http.MethodPost, "/users", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(app, http.MethodGet, "/users/3", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("expected response code 0, got %+v", resp)
	}

	body = bytes.NewBufferString(`{"id":3,"name":"Alice Updated","email":"alice2@example.com"}`)
	w = performRequest(app, http.MethodPut, "/users/3", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if repo.items[3].Name != "Alice Updated" {
		t.Fatalf("expected updated item, got %+v", repo.items[3])
	}

	w = performRequest(app, http.MethodDelete, "/users/3", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if _, ok := repo.items[3]; ok {
		t.Fatalf("expected item deleted")
	}
}

func TestModelViewSetCanUseDefaultGORMRepository(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "app.db")
	cfg.Database.AutoMigrate = true
	cfg.REST.Pagination = true

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE: %v", err)
	}
	defer app.CloseDB()

	app.Register(&ModelViewSet[gormViewSetUser, uint]{
		BasePath:       "/gorm-users",
		SearchFields:   []string{"name", "email"},
		OrderingFields: []string{"id", "name"},
		FilterFields:   []string{"status"},
	})

	body := bytes.NewBufferString(`{"name":"Alice","email":"alice@example.com","status":"active"}`)
	w := performRequest(app, http.MethodPost, "/gorm-users", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(app, http.MethodGet, "/gorm-users?status=active&search=ali&ordering=-id", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var listResp struct {
		Code int `json:"code"`
		Data struct {
			Total   int               `json:"total"`
			Results []gormViewSetUser `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Data.Results) != 1 || listResp.Data.Results[0].Name != "Alice" {
		t.Fatalf("unexpected list response: %+v", listResp.Data.Results)
	}
	if listResp.Data.Total != 1 {
		t.Fatalf("expected total 1, got %d", listResp.Data.Total)
	}

	body = bytes.NewBufferString(`{"id":1,"name":"Alice Updated","email":"alice2@example.com","status":"active"}`)
	w = performRequest(app, http.MethodPut, "/gorm-users/1", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(app, http.MethodGet, "/gorm-users/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var detailResp struct {
		Code int             `json:"code"`
		Data gormViewSetUser `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailResp.Data.Name != "Alice Updated" {
		t.Fatalf("expected updated user, got %+v", detailResp.Data)
	}

	w = performRequest(app, http.MethodDelete, "/gorm-users/1", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(app, http.MethodGet, "/gorm-users/1", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelViewSetDispatchesCollectionExtraActionBesideRetrieve(t *testing.T) {
	app := newTestApp(t)
	app.Register(&ModelViewSet[testUser, uint]{
		BasePath:   "/users",
		Repository: newMemoryUserRepo(),
		ExtraActions: []ExtraAction{
			{
				Method: "get",
				Path:   "export",
				Handler: func(c *Context) {
					JSON(c, http.StatusOK, OK(H{"route": "export"}))
				},
			},
		},
	})

	w := performRequest(app, http.MethodGet, "/users/export", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected collection extra action 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "export") {
		t.Fatalf("expected collection extra action response, got %s", w.Body.String())
	}

	w = performRequest(app, http.MethodGet, "/users/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected retrieve route 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Tom") {
		t.Fatalf("expected retrieve response, got %s", w.Body.String())
	}
}

func TestModelViewSetGORMRepositoryCombinesFiltersAndPagination(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "filter-books.db")
	cfg.Database.AutoMigrate = true
	cfg.REST.Pagination = true

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE: %v", err)
	}
	defer app.CloseDB()

	app.Register(&ModelViewSet[gormFilterBook, uint]{
		BasePath:        "/filter-books",
		OrderingFields:  []string{"id", "name", "year"},
		DefaultOrdering: "-name",
		FilterFields:    []string{"name", "info", "year"},
	})

	for _, body := range []string{
		`{"name":"Book A","info":"aa","year":1111}`,
		`{"name":"Book B","info":"aa","year":1111}`,
		`{"name":"Book C","info":"aa","year":2222}`,
		`{"name":"Book D","info":"bb","year":1111}`,
	} {
		w := performRequest(app, http.MethodPost, "/filter-books", bytes.NewBufferString(body))
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
	}

	w := performRequest(app, http.MethodGet, "/filter-books?page=2&page_size=1&info=aa&year=1111&ordering=id", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var listResp struct {
		Code int `json:"code"`
		Data struct {
			Total    int              `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
			Results  []gormFilterBook `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResp.Data.Page != 2 || listResp.Data.PageSize != 1 {
		t.Fatalf("unexpected pagination response: %+v", listResp.Data)
	}
	if listResp.Data.Total != 2 {
		t.Fatalf("expected filtered total 2, got %d", listResp.Data.Total)
	}
	if len(listResp.Data.Results) != 1 || listResp.Data.Results[0].Name != "Book B" {
		t.Fatalf("unexpected filtered page response: %+v", listResp.Data.Results)
	}

	w = performRequest(app, http.MethodGet, "/filter-books?page=1&page_size=3&year__gte=1111&year__lte=2222&info__in=aa,bb&name__contains=Book&ordering=-year,name", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode operator list response: %v", err)
	}
	if listResp.Data.Total != 4 {
		t.Fatalf("expected operator filtered total 4, got %d", listResp.Data.Total)
	}
	names := []string{}
	for _, book := range listResp.Data.Results {
		names = append(names, book.Name)
	}
	if len(names) != 3 || names[0] != "Book C" || names[1] != "Book A" || names[2] != "Book B" {
		t.Fatalf("unexpected multi-order response: %+v", names)
	}

	w = performRequest(app, http.MethodGet, "/filter-books?page=1&page_size=2&year=1111", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode default ordering response: %v", err)
	}
	if len(listResp.Data.Results) != 2 || listResp.Data.Results[0].Name != "Book D" || listResp.Data.Results[1].Name != "Book B" {
		t.Fatalf("unexpected default ordering response: %+v", listResp.Data.Results)
	}

	w = performRequest(app, http.MethodGet, "/filter-books?email__contains=test", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unlisted filter field, got %d: %s", w.Code, w.Body.String())
	}

	w = performRequest(app, http.MethodGet, "/filter-books?year__between=1111,2222", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported filter operator, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelViewSetUsesDefaultPageConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	cfg.REST.Pagination = true
	cfg.REST.DefaultPage = 3
	cfg.REST.DefaultPageSize = 2
	cfg.REST.MaxPageSize = 10

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE: %v", err)
	}
	repo := newMemoryUserRepo()
	app.Register(&ModelViewSet[testUser, uint]{
		BasePath:   "/users",
		Repository: repo,
	})

	w := performRequest(app, http.MethodGet, "/users", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if repo.lastQuery.Limit != 2 || repo.lastQuery.Offset != 4 {
		t.Fatalf("unexpected default pagination query: %+v", repo.lastQuery)
	}
}

func TestModelViewSetDisablesPaginationByDefault(t *testing.T) {
	app := newTestApp(t)
	repo := newMemoryUserRepo()
	app.Register(&ModelViewSet[testUser, uint]{
		BasePath:   "/users",
		Repository: repo,
	})

	w := performRequest(app, http.MethodGet, "/users?page=3&page_size=1&search=tom&ordering=-id&status=active", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if repo.lastQuery.Limit != 0 || repo.lastQuery.Offset != 0 {
		t.Fatalf("expected pagination to be ignored, got %+v", repo.lastQuery)
	}
	if _, ok := repo.lastQuery.Filters["page"]; ok {
		t.Fatalf("page should not be treated as field filter: %+v", repo.lastQuery.Filters)
	}
	if _, ok := repo.lastQuery.Filters["page_size"]; ok {
		t.Fatalf("page_size should not be treated as field filter: %+v", repo.lastQuery.Filters)
	}

	var listResp struct {
		Code int        `json:"code"`
		Data []testUser `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Data) != 2 {
		t.Fatalf("expected unwrapped full list, got %+v", listResp.Data)
	}
}

func TestModelViewSetCanDisablePaginationWhenGlobalEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	cfg.REST.Pagination = true

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE: %v", err)
	}
	repo := newMemoryUserRepo()
	app.Register(&ModelViewSet[testUser, uint]{
		BasePath:          "/users",
		Repository:        repo,
		DisablePagination: true,
	})

	w := performRequest(app, http.MethodGet, "/users?page=2&page_size=1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if repo.lastQuery.Limit != 0 || repo.lastQuery.Offset != 0 {
		t.Fatalf("expected pagination to be disabled for viewset, got %+v", repo.lastQuery)
	}
}

func TestActionPermissionCanDenyWrite(t *testing.T) {
	app := newTestApp(t)
	app.Register(&ModelViewSet[testUser, uint]{
		BasePath:   "/users",
		Repository: newMemoryUserRepo(),
		ActionPermissions: map[string][]Permission{
			ActionCreate: {ReadOnly{}},
		},
	})

	body := bytes.NewBufferString(`{"name":"Alice"}`)
	w := performRequest(app, http.MethodPost, "/users", body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelViewSetPartialUpdateObjectAndQueryPermissions(t *testing.T) {
	app := newTestApp(t)

	patchRepo := newMemoryUserRepo()
	app.Register(&ModelViewSet[testUser, uint]{
		BasePath:   "/patch-users",
		Repository: patchRepo,
	})

	body := bytes.NewBufferString(`{"id":1,"name":"Tom Patch","email":"tom2@example.com"}`)
	w := performRequest(app, http.MethodPatch, "/patch-users/1", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected PATCH 200, got %d: %s", w.Code, w.Body.String())
	}
	if patchRepo.items[1].Name != "Tom Patch" {
		t.Fatalf("expected patched item, got %+v", patchRepo.items[1])
	}

	objectRepo := newMemoryUserRepo()
	app.Register(&ModelViewSet[testUser, uint]{
		BasePath:          "/object-users",
		Repository:        objectRepo,
		ObjectPermissions: []ObjectPermission[testUser]{denyTestObjectPermission{}},
	})

	w = performRequest(app, http.MethodGet, "/object-users/1", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected object permission 403, got %d: %s", w.Code, w.Body.String())
	}

	queryRepo := newMemoryUserRepo()
	app.Register(&ModelViewSet[testUser, uint]{
		BasePath:         "/query-users",
		Repository:       queryRepo,
		QueryPermissions: []QueryPermission[testUser]{statusScopePermission{}},
	})

	w = performRequest(app, http.MethodGet, "/query-users", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected query permission list 200, got %d: %s", w.Code, w.Body.String())
	}
	if queryRepo.lastQuery.Filters["status"] != "active" {
		t.Fatalf("expected query permission to scope filters, got %+v", queryRepo.lastQuery.Filters)
	}
}

func TestReadOnlyModelViewSetOnlyRegistersReadRoutes(t *testing.T) {
	app := newTestApp(t)
	app.Register(&ReadOnlyModelViewSet[testUser, uint]{
		BasePath:   "/users",
		Repository: newMemoryUserRepo(),
	})

	w := performRequest(app, http.MethodGet, "/users/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	w = performRequest(app, http.MethodPost, "/users", bytes.NewBufferString(`{"name":"Alice"}`))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unregistered write route, got %d", w.Code)
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE: %v", err)
	}
	return app
}

func performRequest(app *App, method, target string, body *bytes.Buffer) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body.Bytes())
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	return w
}
