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

func TestPlanChangeLeavesCachedDetailWhenPersistenceFails(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	changes := NewPlanChangeService(store, cache)
	ctx := context.Background()
	old := domain.NewEntity("sub-a", "tenant-a", "standard")
	if err := store.Save(ctx, "tenant-a/standard", old); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	if err := cache.Put(ctx, "tenant-a", "standard", old); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	store.SetSaveErrorForTest(errors.New("subscription store unavailable"))
	if _, err := changes.Change(ctx, "tenant-a", "standard", "premium"); err == nil {
		t.Fatal("expected persistence failure")
	}
	stored, ok := store.Find(ctx, "tenant-a/standard")
	if !ok || stored.Plan != "standard" {
		t.Fatalf("failed change altered persisted state: %+v found=%t", stored, ok)
	}
	cached, ok := cache.Get(ctx, "tenant-a", "standard")
	if !ok || cached.Plan != "standard" {
		t.Fatalf("failed change discarded the old cached detail: %+v found=%t", cached, ok)
	}
}

// When persistence succeeds but the cache invalidation fails, the persisted
// state must be rolled back so the still-cached detail and the stored state
// keep advertising the original plan. Readers going through the query path
// must observe the same old plan from either source.
func TestPlanChangeRollsBackStateWhenCacheInvalidationFails(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	query := NewEntitlementQueryService(store, cache)
	changes := NewPlanChangeService(store, cache)
	ctx := context.Background()

	old := domain.NewEntity("sub-a", "tenant-a", "standard")
	if err := store.Save(ctx, "tenant-a/standard", old); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	if err := cache.Put(ctx, "tenant-a", "standard", old); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	// Persistence will succeed; only cache retirement fails.
	cache.SetDeleteErrorForTest(errors.New("cache backend unavailable"))
	if _, err := changes.Change(ctx, "tenant-a", "standard", "premium"); err == nil {
		t.Fatal("expected cache invalidation failure to surface")
	}

	stored, ok := store.Find(ctx, "tenant-a/standard")
	if !ok || stored.Plan != "standard" {
		t.Fatalf("rolled-back state must keep the original plan: %+v found=%t", stored, ok)
	}
	cached, ok := cache.Get(ctx, "tenant-a", "standard")
	if !ok || cached.Plan != "standard" {
		t.Fatalf("failed invalidation must leave the original cached detail: %+v found=%t", cached, ok)
	}

	// The query path must read the old plan regardless of which source serves it.
	detail, err := query.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("read detail after failed change: %v", err)
	}
	if detail.Plan != "standard" {
		t.Fatalf("detail after failed change must stay on the original plan: %+v", detail)
	}
}
