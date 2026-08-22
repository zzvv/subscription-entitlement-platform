package application

import (
	"context"
	"testing"

	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
)

func TestEntitlementDetailKeepsCachedProjectionScopedToTenant(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	service := NewEntitlementQueryService(store, cache)
	ctx := context.Background()

	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed tenant-a projection: %v", err)
	}
	if err := store.Save(ctx, "tenant-b/standard", domain.NewEntity("sub-b", "tenant-b", "standard")); err != nil {
		t.Fatalf("seed tenant-b projection: %v", err)
	}
	first, err := service.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("load tenant-a projection: %v", err)
	}
	second, err := service.Detail(ctx, "tenant-b", "standard")
	if err != nil {
		t.Fatalf("load tenant-b projection: %v", err)
	}
	if first.ID == second.ID || second.Tenant != "tenant-b" || second.ID != "sub-b" {
		t.Fatalf("tenant-b received a cached projection from tenant-a: first=%+v second=%+v", first, second)
	}
}
