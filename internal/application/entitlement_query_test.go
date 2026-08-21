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

func TestEntitlementDetailSurvivesCacheWriteFailure(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	service := NewEntitlementQueryService(store, cache)
	ctx := context.Background()

	entity := domain.NewEntity("sub-a", "tenant-a", "standard")
	if err := store.Save(ctx, "tenant-a/standard", entity); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	cache.SetPutErrorForTest(errors.New("cache backend unavailable"))

	got, err := service.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("detail read should survive cache write failure: %v", err)
	}
	if got != entity {
		t.Fatalf("detail returned wrong subscription: got=%+v want=%+v", got, entity)
	}
	// A degraded cache write must leave nothing cached, so a subsequent read
	// still resolves from durable storage rather than a stale half-written
	// entry.
	cache.SetPutErrorForTest(nil)
	if cached, ok := cache.Get(ctx, "tenant-a", "standard"); ok {
		t.Fatalf("cache should remain empty after a degraded write, got=%+v", cached)
	}
	again, err := service.Detail(ctx, "tenant-a", "standard")
	if err != nil || again != entity {
		t.Fatalf("subsequent read after cache degradation: got=%+v err=%v", again, err)
	}
}

func TestEntitlementDetailPropagatesCancellationDuringCacheWrite(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	service := NewEntitlementQueryService(store, cache)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := store.Save(context.Background(), "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	cache.SetPutBeforeLockForTest(cancel)

	_, err := service.Detail(ctx, "tenant-a", "standard")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("detail read must propagate cancellation during cache write, got %v", err)
	}
}
