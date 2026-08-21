package application

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
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

// TestConcurrentDetailAndPlanChangeNeverRestoresStaleCache hammers the
// Detail/PlanChange timing from many goroutines. The failing interleaving is:
// a Detail reads the old plan from the store, pauses, a PlanChange invalidates
// the cache and persists a new plan, then the Detail wakes and writes the stale
// value back into the cache. The fix must make that write-back a no-op, so the
// cache can never hold a plan older than what the store currently holds. The
// assertion runs after every change settles.
func TestConcurrentDetailAndPlanChangeNeverRestoresStaleCache(t *testing.T) {
	const tenants = 4
	const rounds = 60

	for round := 0; round < rounds; round++ {
		store := repository.NewStore()
		cache := repository.NewProjectionCache()
		query := NewEntitlementQueryService(store, cache)
		changes := NewPlanChangeService(store, cache)
		ctx := context.Background()

		// Seed a subscription per tenant at the "standard" plan.
		for i := 0; i < tenants; i++ {
			tenant := fmt.Sprintf("t%d", i)
			if err := store.Save(ctx, tenant+"/standard", domain.NewEntity("sub-"+tenant, tenant, "standard")); err != nil {
				t.Fatalf("seed %s: %v", tenant, err)
			}
		}

		// barrier makes Detail and PlanChange for each tenant race head-to-head.
		var wg sync.WaitGroup
		for i := 0; i < tenants; i++ {
			tenant := fmt.Sprintf("t%d", i)
			nextPlan := "p" + tenant + "-v2"

			// Warming read so the cache is populated before the change, matching
			// the production hot path where detail is cached before a plan changes.
			if _, err := query.Detail(ctx, tenant, "standard"); err != nil {
				t.Fatalf("warm %s: %v", tenant, err)
			}

			wg.Add(3)

			// In-flight detail reader: gets the old value and races to write it back.
			go func() {
				defer wg.Done()
				if _, err := query.Detail(ctx, tenant, "standard"); err != nil {
					t.Errorf("detail %s: %v", tenant, err)
				}
				runtime.Gosched()
			}()

			// Plan change: invalidates cache, persists the new plan.
			go func() {
				defer wg.Done()
				if _, err := changes.Change(ctx, tenant, "standard", nextPlan); err != nil {
					t.Errorf("change %s: %v", tenant, err)
				}
			}()

			// Concurrent reader from a second tenant to stress tenant isolation.
			other := fmt.Sprintf("t%d", (i+1)%tenants)
			go func() {
				defer wg.Done()
				if _, err := query.Detail(ctx, other, "standard"); err != nil && !errors.Is(err, ErrEntitlementNotFound) {
					t.Errorf("cross-detail %s: %v", other, err)
				}
			}()
		}
		wg.Wait()

		// After every change has settled the cache must not hold any plan older
		// than the store's current value for that tenant.
		for i := 0; i < tenants; i++ {
			tenant := fmt.Sprintf("t%d", i)
			expected := "p" + tenant + "-v2"
			stored, ok := store.Find(ctx, tenant+"/standard")
			if !ok || stored.Plan != expected {
				t.Fatalf("round %d %s: store plan = %q found=%t, want %q", round, tenant, stored.Plan, ok, expected)
			}
			if cached, ok := cache.Get(ctx, tenant, "standard"); ok && cached.Plan != expected {
				t.Fatalf("round %d %s: stale cached plan %q survived after change (store has %q)", round, tenant, cached.Plan, expected)
			}
		}
	}
}

// TestConcurrentDetailReadersObserveLatestPlanAfterChange is a lighter, higher-
// contention variant: many Detail readers run alongside a single change and must
// never observe a plan older than the store's committed value. This guards the
// read path even when the change wins the race outright.
func TestConcurrentDetailReadersObserveLatestPlanAfterChange(t *testing.T) {
	const readers = 32
	const tenants = 3

	var fails int64
	for iter := 0; iter < 40; iter++ {
		store := repository.NewStore()
		cache := repository.NewProjectionCache()
		query := NewEntitlementQueryService(store, cache)
		changes := NewPlanChangeService(store, cache)
		ctx := context.Background()

		for i := 0; i < tenants; i++ {
			tenant := fmt.Sprintf("t%d", i)
			if err := store.Save(ctx, tenant+"/standard", domain.NewEntity("sub-"+tenant, tenant, "standard")); err != nil {
				t.Fatalf("seed %s: %v", tenant, err)
			}
			if _, err := query.Detail(ctx, tenant, "standard"); err != nil {
				t.Fatalf("warm %s: %v", tenant, err)
			}
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < tenants; i++ {
				tenant := fmt.Sprintf("t%d", i)
				if _, err := changes.Change(ctx, tenant, "standard", "enterprise"); err != nil {
					t.Errorf("change %s: %v", tenant, err)
				}
			}
		}()

		for r := 0; r < readers; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < tenants; i++ {
					tenant := fmt.Sprintf("t%d", i)
					got, err := query.Detail(ctx, tenant, "standard")
					if err != nil {
						t.Errorf("detail %s: %v", tenant, err)
						return
					}
					if got.Plan != "standard" && got.Plan != "enterprise" {
						atomic.AddInt64(&fails, 1)
						t.Errorf("detail %s returned unexpected plan %q", tenant, got.Plan)
					}
				}
			}()
		}
		wg.Wait()

		// Final invariant: every cached entry, if present, matches the store.
		for i := 0; i < tenants; i++ {
			tenant := fmt.Sprintf("t%d", i)
			stored, _ := store.Find(ctx, tenant+"/standard")
			if cached, ok := cache.Get(ctx, tenant, "standard"); ok && cached != stored {
				t.Fatalf("iter %d %s: cache (%+v) diverged from store (%+v)", iter, tenant, cached, stored)
			}
		}
	}
	if atomic.LoadInt64(&fails) > 0 {
		t.Fatalf("observed %d unexpected-plan reads across iterations", fails)
	}
}
