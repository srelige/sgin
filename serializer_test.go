package sgin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"testing"
)

type serializerAccount struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	IsAdmin      bool   `json:"is_admin"`
	OwnerID      uint   `json:"owner_id"`
	Enabled      bool   `json:"enabled"`
}

type serializerAccountRepo struct {
	items map[uint]serializerAccount
}

func newSerializerAccountRepo() *serializerAccountRepo {
	return &serializerAccountRepo{
		items: map[uint]serializerAccount{
			1: {
				ID:           1,
				Name:         "Tom",
				Email:        "tom@example.com",
				PasswordHash: "hashed",
				IsAdmin:      true,
				OwnerID:      100,
				Enabled:      true,
			},
		},
	}
}

func (r *serializerAccountRepo) List(ctx context.Context, q Query) ([]serializerAccount, error) {
	keys := make([]int, 0, len(r.items))
	for id := range r.items {
		keys = append(keys, int(id))
	}
	sort.Ints(keys)

	out := make([]serializerAccount, 0, len(keys))
	for _, id := range keys {
		out = append(out, r.items[uint(id)])
	}
	return out, nil
}

func (r *serializerAccountRepo) Find(ctx context.Context, id uint) (*serializerAccount, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &item, nil
}

func (r *serializerAccountRepo) Create(ctx context.Context, obj *serializerAccount) error {
	if obj.ID == 0 {
		obj.ID = uint(len(r.items) + 1)
	}
	r.items[obj.ID] = *obj
	return nil
}

func (r *serializerAccountRepo) Update(ctx context.Context, obj *serializerAccount) error {
	r.items[obj.ID] = *obj
	return nil
}

func (r *serializerAccountRepo) Delete(ctx context.Context, id uint) error {
	delete(r.items, id)
	return nil
}

func TestModelViewSetRequiresSerializer(t *testing.T) {
	app := newTestApp(t)
	defer func() {
		if recover() == nil {
			t.Fatal("expected missing serializer to panic")
		}
	}()

	app.Register(&ModelViewSet[serializerAccount, uint]{
		BasePath:   "/accounts",
		Repository: newSerializerAccountRepo(),
	})
}

func TestModelViewSetRequiresWriteStrategyForWritableModelSerializer(t *testing.T) {
	app := newTestApp(t)
	defer func() {
		if recover() == nil {
			t.Fatal("expected writable ModelSerializer without write strategy to panic")
		}
	}()

	app.Register(&ModelViewSet[serializerAccount, uint]{
		BasePath:   "/accounts",
		Repository: newSerializerAccountRepo(),
		Serializer: ModelSerializer[serializerAccount]{
			ReadFields: []string{"id", "name"},
		},
	})
}

func TestModelSerializerFiltersReadAndWriteFields(t *testing.T) {
	app := newTestApp(t)
	repo := newSerializerAccountRepo()
	app.Register(&ModelViewSet[serializerAccount, uint]{
		BasePath:   "/accounts",
		Repository: repo,
		Serializer: ModelSerializer[serializerAccount]{
			ReadFields:  []string{"id", "name", "email"},
			WriteFields: []string{"name", "email"},
		},
	})

	w := performRequest(app, http.MethodPost, "/accounts", bytes.NewBufferString(`{"name":"Alice","email":"alice@example.com"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Data["name"] != "Alice" || created.Data["email"] != "alice@example.com" {
		t.Fatalf("unexpected create response: %+v", created.Data)
	}
	if _, ok := created.Data["is_admin"]; ok {
		t.Fatalf("read response should not expose is_admin: %+v", created.Data)
	}
	if repo.items[2].IsAdmin || repo.items[2].OwnerID != 0 || repo.items[2].Enabled {
		t.Fatalf("unexpected sensitive fields written: %+v", repo.items[2])
	}

	w = performRequest(app, http.MethodGet, "/accounts/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var retrieved struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &retrieved); err != nil {
		t.Fatalf("decode retrieve response: %v", err)
	}
	for _, field := range []string{"password_hash", "is_admin", "owner_id", "enabled"} {
		if _, ok := retrieved.Data[field]; ok {
			t.Fatalf("read response should not expose %s: %+v", field, retrieved.Data)
		}
	}
}

func TestModelSerializerRejectsUndeclaredWriteFields(t *testing.T) {
	for field, payload := range map[string]string{
		"is_admin": `{"name":"Alice","email":"alice@example.com","is_admin":true}`,
		"owner_id": `{"name":"Alice","email":"alice@example.com","owner_id":1}`,
		"enabled":  `{"name":"Alice","email":"alice@example.com","enabled":true}`,
	} {
		t.Run(field, func(t *testing.T) {
			app := newTestApp(t)
			repo := newSerializerAccountRepo()
			app.Register(&ModelViewSet[serializerAccount, uint]{
				BasePath:   "/accounts",
				Repository: repo,
				Serializer: ModelSerializer[serializerAccount]{
					ReadFields:  []string{"id", "name", "email"},
					WriteFields: []string{"name", "email"},
				},
			})

			w := performRequest(app, http.MethodPost, "/accounts", bytes.NewBufferString(payload))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			if len(repo.items) != 1 {
				t.Fatalf("unexpected item created after rejected field: %+v", repo.items)
			}
		})
	}
}

func TestModelSerializerExcludeFields(t *testing.T) {
	app := newTestApp(t)
	app.Register(&ModelViewSet[serializerAccount, uint]{
		BasePath:   "/accounts",
		Repository: newSerializerAccountRepo(),
		Serializer: ModelSerializer[serializerAccount]{
			ExcludeFields: []string{"password_hash", "is_admin", "owner_id", "enabled"},
		},
	})

	w := performRequest(app, http.MethodGet, "/accounts/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var retrieved struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &retrieved); err != nil {
		t.Fatalf("decode retrieve response: %v", err)
	}
	for _, field := range []string{"id", "name", "email"} {
		if _, ok := retrieved.Data[field]; !ok {
			t.Fatalf("expected %s in response: %+v", field, retrieved.Data)
		}
	}
	for _, field := range []string{"password_hash", "is_admin", "owner_id", "enabled"} {
		if _, ok := retrieved.Data[field]; ok {
			t.Fatalf("response should exclude %s: %+v", field, retrieved.Data)
		}
	}

	w = performRequest(app, http.MethodPost, "/accounts", bytes.NewBufferString(`{"name":"Alice","password_hash":"plain"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for excluded write field, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReadOnlyModelSerializerDoesNotRequireWriteFields(t *testing.T) {
	app := newTestApp(t)
	app.Register(&ReadOnlyModelViewSet[serializerAccount, uint]{
		BasePath:   "/accounts",
		Repository: newSerializerAccountRepo(),
		Serializer: ModelSerializer[serializerAccount]{
			ReadFields: []string{"id", "name"},
		},
	})

	w := performRequest(app, http.MethodGet, "/accounts/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var retrieved struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &retrieved); err != nil {
		t.Fatalf("decode retrieve response: %v", err)
	}
	if _, ok := retrieved.Data["email"]; ok {
		t.Fatalf("read-only serializer should honor read fields: %+v", retrieved.Data)
	}
}
