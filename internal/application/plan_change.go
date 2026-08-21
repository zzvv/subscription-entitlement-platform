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

// Change amends the subscription plan. It invalidates the cached detail before
// persisting so a cache-backend failure leaves both store and cache untouched,
// then advances the cache's version high-water mark to the committed version. The
// high-water mark makes any in-flight Detail that read the old plan drop its stale
// write-back, and evicts a stale entry that slipped back in between the two steps.
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
	if err := s.cache.Delete(ctx, tenant, scope); err != nil {
		return domain.Entity{}, err
	}
	current.Plan = plan
	if err := s.store.Save(ctx, key, current); err != nil {
		return domain.Entity{}, err
	}
	_, committedVersion, _ := s.store.FindWithVersion(ctx, key)
	if err := s.cache.AdvanceVersion(ctx, tenant, scope, committedVersion); err != nil {
		return domain.Entity{}, err
	}
	return current, nil
}
