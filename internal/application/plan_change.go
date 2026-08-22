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
	previous, hadPrevious := s.cache.Get(ctx, tenant, scope)
	if err := s.cache.Delete(ctx, tenant, scope); err != nil {
		return domain.Entity{}, err
	}
	current.Plan = plan
	if err := s.store.Save(ctx, key, current); err != nil {
		if hadPrevious {
			_ = s.cache.Put(ctx, tenant, scope, previous)
		}
		return domain.Entity{}, err
	}
	return current, nil
}
