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
