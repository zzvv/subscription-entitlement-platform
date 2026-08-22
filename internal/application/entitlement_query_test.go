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

// TestEntitlementDetailReadsNewPlanAfterChange 复现并守住"计划变更后详情不再读到旧缓存"。
// 旧实现里 PlanChangeService.Change 只写 store、不清缓存，EntitlementQueryService.Detail
// 会先命中旧投影，把已失效的旧权益返回给审核人员。该测试在修复前稳定失败、修复后稳定通过。
func TestEntitlementDetailReadsNewPlanAfterChange(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	query := NewEntitlementQueryService(store, cache)
	changes := NewPlanChangeService(store, cache)
	ctx := context.Background()

	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	// 预热详情缓存，确保后续查询会先走缓存命中旧权益（复现旧 bug 的前提）。
	warm, err := query.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("warm detail cache: %v", err)
	}
	if warm.Plan != "standard" {
		t.Fatalf("unexpected seeded plan: %+v", warm)
	}

	if _, err := changes.Change(ctx, "tenant-a", "standard", "enterprise"); err != nil {
		t.Fatalf("change plan: %v", err)
	}

	// 变更成功后，紧接的下一次详情必须拿到新权益，而不是旧缓存里的 standard。
	detail, err := query.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("detail after change: %v", err)
	}
	if detail.Plan != "enterprise" {
		t.Fatalf("detail served stale cached plan: got %q want %q", detail.Plan, "enterprise")
	}
	// 再次查询确认新权益被正确回填缓存，而非依赖每次穿透 store。
	reload, err := query.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("reload detail: %v", err)
	}
	if reload.Plan != "enterprise" {
		t.Fatalf("reloaded detail served stale cached plan: got %q want %q", reload.Plan, "enterprise")
	}
}

// TestEntitlementDetailCacheInvalidationScopedToTenant 确保失效只针对被改租户，
// 不会误清其他租户的详情缓存——租户隔离行为保持不变。
func TestEntitlementDetailCacheInvalidationScopedToTenant(t *testing.T) {
	store := repository.NewStore()
	cache := repository.NewProjectionCache()
	query := NewEntitlementQueryService(store, cache)
	changes := NewPlanChangeService(store, cache)
	ctx := context.Background()

	if err := store.Save(ctx, "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed tenant-a: %v", err)
	}
	if err := store.Save(ctx, "tenant-b/standard", domain.NewEntity("sub-b", "tenant-b", "standard")); err != nil {
		t.Fatalf("seed tenant-b: %v", err)
	}

	// 两个租户的详情缓存都已预热。
	if _, err := query.Detail(ctx, "tenant-a", "standard"); err != nil {
		t.Fatalf("warm tenant-a: %v", err)
	}
	if _, err := query.Detail(ctx, "tenant-b", "standard"); err != nil {
		t.Fatalf("warm tenant-b: %v", err)
	}

	if _, err := changes.Change(ctx, "tenant-a", "standard", "enterprise"); err != nil {
		t.Fatalf("change tenant-a plan: %v", err)
	}

	// tenant-a 必须读到新权益。
	aDetail, err := query.Detail(ctx, "tenant-a", "standard")
	if err != nil {
		t.Fatalf("detail tenant-a: %v", err)
	}
	if aDetail.Plan != "enterprise" {
		t.Fatalf("tenant-a served stale plan: got %q want %q", aDetail.Plan, "enterprise")
	}
	// tenant-b 未被改动，其缓存仍应命中、且保持原值（行为不变）。
	bDetail, err := query.Detail(ctx, "tenant-b", "standard")
	if err != nil {
		t.Fatalf("detail tenant-b: %v", err)
	}
	if bDetail.Tenant != "tenant-b" || bDetail.Plan != "standard" || bDetail.ID != "sub-b" {
		t.Fatalf("tenant-b projection disturbed by tenant-a change: %+v", bDetail)
	}
}
