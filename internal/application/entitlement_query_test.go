package application

import (
	"context"
	"errors"
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
	// Tenant A opens the detail first; the projection is cached under its key.
	first, err := service.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("load tenant-a projection: %v", err)
	}
	// Tenant B then opens the same scope. The cache must NOT serve tenant-a's
	// entry; it must miss and fall back to the store for tenant-b's projection.
	second, err := service.Detail(ctx, "tenant-b", "standard")
	if err != nil {
		t.Fatalf("load tenant-b projection: %v", err)
	}
	if first.ID == second.ID || second.Tenant != "tenant-b" || second.ID != "sub-b" {
		t.Fatalf("tenant-b received a cached projection from tenant-a: first=%+v second=%+v", first, second)
	}

	// Re-querying tenant-a must still return tenant-a's projection (cache hit
	// for the same tenant), proving the cache is keyed by tenant+scope rather
	// than dropped entirely.
	retry, err := service.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("reload tenant-a projection: %v", err)
	}
	if retry.ID != "sub-a" || retry.Tenant != "tenant-a" {
		t.Fatalf("tenant-a cache hit returned wrong projection: %+v", retry)
	}

	// A tenant with no projection must miss the cache and the store, surfacing
	// the not-found error rather than leaking another tenant's cached entry.
	if _, err := service.Detail(ctx, "tenant-c", "standard"); !errors.Is(err, ErrEntitlementNotFound) {
		t.Fatalf("expected ErrEntitlementNotFound for unknown tenant, got %v", err)
	}
}
