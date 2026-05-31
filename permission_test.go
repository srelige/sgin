package sgin

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuiltinPermissions(t *testing.T) {
	c := testPermissionContext()

	if !(AllowAny{}).HasPermission(c, ActionList).Allowed {
		t.Fatal("expected AllowAny to allow")
	}
	if (IsAuthenticated{}).HasPermission(c, ActionList).Allowed {
		t.Fatal("expected missing user to be unauthenticated")
	}
	if decision := (IsAuthenticated{}).HasPermission(c, ActionList); decision.Code != ErrCodeAuthenticationRequired {
		t.Fatalf("expected authentication_required, got %+v", decision)
	}
	if decision := (IsAdmin{}).HasPermission(c, ActionList); decision.Allowed || decision.Code != ErrCodeAuthenticationRequired {
		t.Fatal("expected missing user to fail admin permission")
	}

	c.Set("user", UserAccount{ID: 1, Username: "user", Enabled: true})
	if !(IsAuthenticated{}).HasPermission(c, ActionList).Allowed {
		t.Fatal("expected user context to pass authentication")
	}
	if (IsAdmin{}).HasPermission(c, ActionList).Allowed {
		t.Fatal("expected non-admin user to fail admin permission")
	}

	c.Set("user", UserAccount{ID: 1, Username: "admin", Admin: true, Enabled: true})
	if !(IsAdmin{}).HasPermission(c, ActionList).Allowed {
		t.Fatal("expected admin user to pass admin permission")
	}

	if !(ReadOnly{}).HasPermission(c, ActionList).Allowed || !(ReadOnly{}).HasPermission(c, ActionRetrieve).Allowed {
		t.Fatal("expected read actions to pass readonly permission")
	}
	if (ReadOnly{}).HasPermission(c, ActionCreate).Allowed {
		t.Fatal("expected write action to fail readonly permission")
	}
	if decision := (ReadOnly{}).HasPermission(c, ActionCreate); decision.Code != ErrCodePermissionDenied {
		t.Fatalf("expected permission_denied, got %+v", decision)
	}

	decision := denyPermission{decision: Deny("custom", "custom denied")}.HasPermission(c, ActionList)
	if decision.Allowed || decision.Code != "custom" {
		t.Fatalf("unexpected deny permission decision: %+v", decision)
	}
}

func TestComposedPermissions(t *testing.T) {
	c := testPermissionContext()

	if !And(AllowAny{}, nil).HasPermission(c, ActionList).Allowed {
		t.Fatal("expected And to ignore nil and allow")
	}
	if And(AllowAny{}, ReadOnly{}).HasPermission(c, ActionCreate).Allowed {
		t.Fatal("expected And to return first denied decision")
	}
	if !Or(ReadOnly{}, AllowAny{}).HasPermission(c, ActionCreate).Allowed {
		t.Fatal("expected Or to allow when any child allows")
	}
	if Or().HasPermission(c, ActionList).Allowed {
		t.Fatal("expected empty Or to deny")
	}
	if Or(nil).HasPermission(c, ActionList).Allowed {
		t.Fatal("expected Or with only nil permissions to deny")
	}
	if Not(AllowAny{}).HasPermission(c, ActionList).Allowed {
		t.Fatal("expected Not to deny allowed child")
	}
	if !Not(ReadOnly{}).HasPermission(c, ActionCreate).Allowed {
		t.Fatal("expected Not to allow denied child")
	}
	if !Not(nil).HasPermission(c, ActionList).Allowed {
		t.Fatal("expected Not nil to allow")
	}
}

func TestPermissionRegistry(t *testing.T) {
	registry := NewPermissionRegistry()
	if perm, ok := registry.Make("allow_any"); !ok || perm == nil {
		t.Fatal("expected builtin permission from registry")
	}
	if _, ok := registry.Make("missing"); ok {
		t.Fatal("expected missing permission lookup to fail")
	}

	registry.Register("custom", func() Permission { return ReadOnly{} })
	perm, ok := registry.Make("custom")
	if !ok || !perm.HasPermission(testPermissionContext(), ActionList).Allowed {
		t.Fatal("expected custom permission to be created")
	}

	registry.Register("", func() Permission { return AllowAny{} })
	registry.Register("nil", nil)
	if _, ok := registry.Make(""); ok {
		t.Fatal("expected empty permission name to be ignored")
	}
	if _, ok := registry.Make("nil"); ok {
		t.Fatal("expected nil permission factory to be ignored")
	}

	var nilRegistry *PermissionRegistry
	nilRegistry.Register("ignored", func() Permission { return AllowAny{} })
	if _, ok := nilRegistry.Make("ignored"); ok {
		t.Fatal("expected nil registry lookup to fail")
	}
}

func testPermissionContext() *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c
}
