package application

import (
	"context"
	"errors"

	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
)

var ErrSubscriptionNotFound = errors.New("subscription not found")

// PlanChangeService applies a subscription amendment and retires its old detail projection.
type PlanChangeService struct {
	store *repository.Store
	cache *repository.ProjectionCache
}

func NewPlanChangeService(store *repository.Store, cache *repository.ProjectionCache) *PlanChangeService {
	return &PlanChangeService{store: store, cache: cache}
}

func (s *PlanChangeService) Change(ctx context.Context, tenant, scope, plan string) (domain.Entity, error) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, err
	}
	key := tenant + "/" + scope
	current, ok := s.store.Find(ctx, key)
	if !ok {
		if err := ctx.Err(); err != nil {
			return domain.Entity{}, err
		}
		return domain.Entity{}, ErrSubscriptionNotFound
	}
	// Invalidate the cached detail projection before persisting the new plan.
	// Order matters: if invalidation fails, neither the subscription state nor
	// the stale detail must be touched, so callers never observe a half-applied
	// amendment (new plan in the store but old entitlements still cached). When
	// invalidation succeeds and the subsequent Save fails, the next detail query
	// simply re-reads and re-caches the unchanged old state, keeping the two
	// consistent. Only a fully successful invalidate+save exposes new entitlements.
	if err := s.cache.Delete(ctx, tenant, scope); err != nil {
		return domain.Entity{}, err
	}
	current.Plan = plan
	if err := s.store.Save(ctx, key, current); err != nil {
		return domain.Entity{}, err
	}
	return current, nil
}
