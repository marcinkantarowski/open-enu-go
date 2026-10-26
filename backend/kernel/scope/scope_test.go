package scope

import (
	"context"
	"testing"
)

func TestTenantInContext(t *testing.T) {
	ctx := context.Background()
	if id, ok := TenantID(ctx); ok || id != "" {
		t.Fatalf("bare context has no tenant, got %q %v", id, ok)
	}
	if TenantOrGlobal(ctx) != "global" {
		t.Fatal("unscoped is 'global'")
	}
	ctx = WithTenant(ctx, "t1")
	if id, ok := TenantID(ctx); !ok || id != "t1" {
		t.Fatalf("got %q %v", id, ok)
	}
	if _, ok := TenantID(WithTenant(ctx, "")); ok {
		t.Fatal("an empty tenant id is no tenant")
	}
}

func TestUnscopedLeavesNoTenantBehind(t *testing.T) {
	ctx := WithTenant(context.Background(), "t1")
	err := Unscoped(ctx, "test: the ambient tenant must not leak inside", func(ctx context.Context) error {
		if id, ok := TenantID(ctx); ok {
			t.Fatalf("the request's tenant %q is still visible inside Unscoped", id)
		}
		if !isBypass(ctx) {
			t.Fatal("no bypass inside Unscoped")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := TenantID(ctx); id != "t1" {
		t.Fatal("the outer context changed")
	}
}
