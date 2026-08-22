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
	// Persist first so a failed write leaves both the stored state and the
	// cached detail projection untouched. The cache only retires once the new
	// state is durable; a failure there rolls the state back to keep them in
	// step with the still-cached detail.
	updated := current
	updated.Plan = plan
	if err := s.store.Save(ctx, key, updated); err != nil {
		return domain.Entity{}, err
	}
	if err := s.cache.Delete(ctx, tenant, scope); err != nil {
		// The new state is durable but its detail cache could not be retired.
		// Restore the prior state so readers still observe the original plan
		// served from cache, instead of a newer plan the cache never knew about.
		if rbErr := s.store.Save(ctx, key, current); rbErr != nil {
			return domain.Entity{}, errors.Join(err, rbErr)
		}
		return domain.Entity{}, err
	}
	return updated, nil
}
