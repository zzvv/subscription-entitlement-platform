package application

import (
	"context"
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
