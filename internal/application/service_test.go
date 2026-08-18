package application

import (
	"context"
	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
	"testing"
)

func TestEntitlementsAreIsolatedBySubscription(t *testing.T) {
	store := repository.NewStore()
	service := NewService(store)
	ctx := context.Background()
	first := domain.NewCommand("sub-a", "tenant-a", "standard", "activate")
	second := domain.NewCommand("sub-b", "tenant-b", "standard", "activate")
	if _, err := service.Process(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Process(ctx, second); err != nil {
		t.Fatal(err)
	}
	got, ok := store.Find(ctx, second.Tenant+"/"+second.Scope)
	if !ok || got.Tenant != second.Tenant {
		t.Fatalf("subscription isolation failed: %#v", got)
	}
}

// TestCrossTenantCacheDoesNotLeakEntitlement asserts that once tenant-a has an
// active entitlement for the "standard" scope, tenant-b activating the same
// scope name receives its own entitlement rather than tenant-a's cached entry.
func TestCrossTenantCacheDoesNotLeakEntitlement(t *testing.T) {
	store := repository.NewStore()
	service := NewService(store)
	ctx := context.Background()

	first := domain.NewCommand("sub-a", "tenant-a", "standard", "activate")
	if got, err := service.Process(ctx, first); err != nil || got.Tenant != "tenant-a" {
		t.Fatalf("first activate: got=%#v err=%v", got, err)
	}

	// tenant-b reuses the same scope name; before the fix this hit
	// tenant-a's cache entry and returned the wrong tenant.
	second := domain.NewCommand("sub-b", "tenant-b", "standard", "activate")
	got, err := service.Process(ctx, second)
	if err != nil {
		t.Fatalf("second activate: %v", err)
	}
	if got.Tenant != "tenant-b" {
		t.Fatalf("cross-tenant leak: expected tenant-b, got %q (%#v)", got.Tenant, got)
	}
	if got.ID != "sub-b" {
		t.Fatalf("cross-tenant leak: expected id sub-b, got %q", got.ID)
	}
}

// TestSameTenantEntitlementIsCached verifies that a second activate for the
// same tenant and scope still resolves to the tenant's own cached entitlement,
// i.e. caching is preserved within a tenant boundary.
func TestSameTenantEntitlementIsCached(t *testing.T) {
	store := repository.NewStore()
	service := NewService(store)
	ctx := context.Background()

	first := domain.NewCommand("sub-a", "tenant-a", "standard", "activate")
	gotFirst, err := service.Process(ctx, first)
	if err != nil {
		t.Fatalf("first activate: %v", err)
	}

	second := domain.NewCommand("sub-a", "tenant-a", "standard", "activate")
	gotSecond, err := service.Process(ctx, second)
	if err != nil {
		t.Fatalf("second activate: %v", err)
	}
	if gotSecond.Tenant != "tenant-a" {
		t.Fatalf("cached entitlement lost tenant: %q", gotSecond.Tenant)
	}
	if gotSecond.ID != gotFirst.ID {
		t.Fatalf("expected cached entitlement id %q, got %q", gotFirst.ID, gotSecond.ID)
	}
}
