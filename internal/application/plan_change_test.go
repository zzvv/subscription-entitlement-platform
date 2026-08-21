package application

import (
	"context"
	"errors"
	"testing"

	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
)

func TestPlanChangeInvalidatesCachedEntitlementDetail(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	query := NewEntitlementQueryService(store, cache)
	changes := NewPlanChangeService(store, cache)
	ctx := context.Background()

	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	first, err := query.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("warm entitlement detail cache: %v", err)
	}
	if first.Plan != "standard" {
		t.Fatalf("unexpected initial plan: %+v", first)
	}
	changed, err := changes.Change(ctx, "tenant-a", "standard", "enterprise")
	if err != nil {
		t.Fatalf("change subscription plan: %v", err)
	}
	if changed.Plan != "enterprise" {
		t.Fatalf("change returned wrong plan: %+v", changed)
	}
	detail, err := query.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("reload entitlement detail: %v", err)
	}
	if detail.Plan != "enterprise" {
		t.Fatalf("detail retained stale cached plan after change: %+v", detail)
	}
}

func TestPlanChangeLeavesStateAndCacheUntouchedWhenInvalidationFails(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	query := NewEntitlementQueryService(store, cache)
	changes := NewPlanChangeService(store, cache)
	ctx := context.Background()

	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := query.Detail(ctx, "tenant-a", "standard"); err != nil {
		t.Fatalf("warm entitlement detail cache: %v", err)
	}
	cache.SetDeleteErrorForTest(errors.New("cache backend unavailable"))
	if _, err := changes.Change(ctx, "tenant-a", "standard", "enterprise"); err == nil {
		t.Fatal("expected cache invalidation failure")
	}
	stored, ok := store.Find(ctx, "tenant-a/standard")
	if !ok || stored.Plan != "standard" {
		t.Fatalf("failed change must not persist a new plan: %+v found=%t", stored, ok)
	}
	cached, ok := cache.Get(ctx, "tenant-a", "standard")
	if !ok || cached.Plan != "standard" {
		t.Fatalf("failed change must keep the original cached detail: %+v found=%t", cached, ok)
	}
}

func TestCancelledPlanChangeDoesNotLeaveCacheInvalidated(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	query := NewEntitlementQueryService(store, cache)
	changes := NewPlanChangeService(store, cache)
	ctx, cancel := context.WithCancel(context.Background())
	if err := store.Save(context.Background(), "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := query.Detail(context.Background(), "tenant-a", "standard"); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	store.SetSaveBeforeLockForTest(cancel)
	if _, err := changes.Change(ctx, "tenant-a", "standard", "enterprise"); err != nil {
		t.Fatalf("plan change unexpectedly failed: %v", err)
	}
	if _, ok := cache.Get(context.Background(), "tenant-a", "standard"); !ok {
		t.Fatal("cancelled plan change must not leave the cache invalidated")
	}
}
