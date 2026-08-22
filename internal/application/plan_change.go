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
	current.Plan = plan
	if err := s.store.Save(ctx, key, current); err != nil {
		return domain.Entity{}, err
	}
	// Save 持久化新权益后再失效详情投影：确保下一次查询命中不到旧缓存，
	// 只能回 store 取到刚写入的新权益并重新回填。Delete 失败则视为变更未完成，
	// 不向上游报告成功，避免审核期间读到旧权益。
	if err := s.cache.Delete(ctx, tenant, scope); err != nil {
		return domain.Entity{}, err
	}
	return current, nil
}
