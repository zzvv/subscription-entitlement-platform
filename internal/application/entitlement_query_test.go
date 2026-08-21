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

func TestPlanChangeDoesNotAllowInFlightDetailToRestoreStaleCache(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	query := NewEntitlementQueryService(store, cache)
	changes := NewPlanChangeService(store, cache)
	ctx := context.Background()
	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	query.SetBeforeCachePutForTest(func() {
		close(entered)
		<-release
	})
	result := make(chan error, 1)
	go func() {
		_, err := query.Detail(ctx, "tenant-a", "standard")
		result <- err
	}()
	<-entered
	if _, err := changes.Change(ctx, "tenant-a", "standard", "enterprise"); err != nil {
		t.Fatalf("change plan: %v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("in-flight detail: %v", err)
	}
	cached, ok := cache.Get(ctx, "tenant-a", "standard")
	if ok && cached.Plan != "enterprise" {
		t.Fatalf("in-flight detail restored stale cached plan after change: %+v", cached)
	}
}
