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

// TestPlanChangeLeavesStateAndCacheUntouchedWhenSaveFails covers the second
// half-finished path introduced by invalidating the cache before persisting:
// the cache invalidation succeeds but the subsequent store Save fails. The
// subscription state must remain the old plan and the next detail query must
// still report the old entitlements (the cache was emptied, so the query
// re-reads the unchanged old state from the store and re-caches it).
func TestPlanChangeLeavesStateAndCacheUntouchedWhenSaveFails(t *testing.T) {
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
	store.SetSaveErrorForTest(errors.New("store backend unavailable"))
	if _, err := changes.Change(ctx, "tenant-a", "standard", "enterprise"); err == nil {
		t.Fatal("expected store save failure")
	}
	stored, ok := store.Find(ctx, "tenant-a/standard")
	if !ok || stored.Plan != "standard" {
		t.Fatalf("failed change must not persist a new plan: %+v found=%t", stored, ok)
	}
	// Cache was invalidated before the failed save, so the next detail query
	// must rebuild the projection from the untouched old subscription state.
	detail, err := query.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("reload entitlement detail after failed change: %v", err)
	}
	if detail.Plan != "standard" {
		t.Fatalf("failed change must keep entitlements at the old plan: %+v", detail)
	}
	cached, ok := cache.Get(ctx, "tenant-a", "standard")
	if !ok || cached.Plan != "standard" {
		t.Fatalf("cache must be repopulated with the old detail: %+v found=%t", cached, ok)
	}
}

// TestPlanChangeExposesNewEntitlementsOnlyAfterFullSuccess guards the success
// invariant: a fully completed change must leave the subscription state on the
// new plan and the subsequent detail query must surface the new entitlements
// (cache repopulated from the updated store), never a stale cached copy.
func TestPlanChangeExposesNewEntitlementsOnlyAfterFullSuccess(t *testing.T) {
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
	if _, err := changes.Change(ctx, "tenant-a", "standard", "enterprise"); err != nil {
		t.Fatalf("change subscription plan: %v", err)
	}
	stored, ok := store.Find(ctx, "tenant-a/standard")
	if !ok || stored.Plan != "enterprise" {
		t.Fatalf("successful change must persist the new plan: %+v found=%t", stored, ok)
	}
	if _, ok := cache.Get(ctx, "tenant-a", "standard"); ok {
		t.Fatal("successful change must retire the stale cached detail before the next query")
	}
	detail, err := query.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("reload entitlement detail after change: %v", err)
	}
	if detail.Plan != "enterprise" {
		t.Fatalf("detail must expose the new entitlements after change: %+v", detail)
	}
}
