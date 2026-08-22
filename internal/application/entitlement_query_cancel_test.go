package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
)

// TestEntitlementDetailDoesNotReturnEntityWhenCanceledWhileWaitingForReadLock
// reproduces the boundary case where a detail query is canceled while it is
// parked waiting for the Store read lock. The canceled request must surface a
// context error instead of the subscription entity and must not warm the
// detail cache.
func TestEntitlementDetailDoesNotReturnEntityWhenCanceledWhileWaitingForReadLock(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	service := NewEntitlementQueryService(store, cache)
	ctx := context.Background()

	// Seed a projection that would otherwise be returned as a successful hit.
	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	store.SetFindBeforeLockForTest(func() {
		close(entered)
		<-release
	})

	queryCtx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		entity domain.Entity
		err    error
	}
	result := make(chan outcome, 1)
	go func() {
		entity, err := service.Detail(queryCtx, "tenant-a", "standard")
		result <- outcome{entity: entity, err: err}
	}()

	// Wait until Detail is parked inside Store.Find waiting for the read lock,
	// then cancel the request before letting it proceed.
	<-entered
	cancel()
	close(release)

	got := <-result
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled detail must return a context error, got entity=%+v err=%v", got.entity, got.err)
	}
	if got.entity != (domain.Entity{}) {
		t.Fatalf("canceled detail must not return a subscription entity, got %+v", got.entity)
	}

	// The canceled result must never be written into the detail cache. A
	// follow-up query for the same projection should be served from Store and
	// remain untouched, not poisoned by the canceled attempt.
	if _, ok := cache.Get(context.Background(), "tenant-a", "standard"); ok {
		t.Fatal("canceled detail must not warm the projection cache")
	}
}

// TestEntitlementDetailStillServesNormalQueryAfterFix guards against
// regressions in the happy path: an uncanceled detail query must return the
// seeded projection and warm the cache.
func TestEntitlementDetailStillServesNormalQueryAfterFix(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	service := NewEntitlementQueryService(store, cache)
	ctx := context.Background()

	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	got, err := service.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("normal detail query failed: %v", err)
	}
	if got.ID != "sub-a" || got.Tenant != "tenant-a" {
		t.Fatalf("unexpected detail entity: %+v", got)
	}
	if _, ok := cache.Get(ctx, "tenant-a", "standard"); !ok {
		t.Fatal("normal detail query should warm the projection cache")
	}
}

// TestEntitlementDetailPreservesTenantIsolationAfterFix ensures the cancel
// handling does not disturb tenant scoping of the detail result and cache.
func TestEntitlementDetailPreservesTenantIsolationAfterFix(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	service := NewEntitlementQueryService(store, cache)
	ctx := context.Background()

	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed tenant-a: %v", err)
	}
	if err := store.Save(ctx, "tenant-b/standard", domain.NewEntity("sub-b", "tenant-b", "standard")); err != nil {
		t.Fatalf("seed tenant-b: %v", err)
	}

	a, err := service.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("load tenant-a: %v", err)
	}
	b, err := service.Detail(ctx, "tenant-b", "standard")
	if err != nil {
		t.Fatalf("load tenant-b: %v", err)
	}
	if a.Tenant != "tenant-a" || b.Tenant != "tenant-b" || a.ID == b.ID {
		t.Fatalf("tenant isolation broken: a=%+v b=%+v", a, b)
	}
}

// TestEntitlementDetailReturnsPromptlyWhenContextAlreadyCanceled covers the
// path where the context is already canceled before Detail reaches the store,
// ensuring it returns promptly without waiting on lock contention.
func TestEntitlementDetailReturnsPromptlyWhenContextAlreadyCanceled(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	service := NewEntitlementQueryService(store, cache)
	ctx := context.Background()

	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		_, _ = service.Detail(canceled, "tenant-a", "standard")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("detail with an already-canceled context must return promptly")
	}
}
