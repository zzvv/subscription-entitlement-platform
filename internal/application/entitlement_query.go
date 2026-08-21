package application

import (
	"context"
	"errors"

	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
)

var ErrEntitlementNotFound = errors.New("entitlement projection not found")

// EntitlementQueryService serves the material-detail subscription projection.
type EntitlementQueryService struct {
	store          *repository.Store
	cache          *repository.ProjectionCache
	beforeCachePut func()
}

func (s *EntitlementQueryService) SetBeforeCachePutForTest(hook func()) {
	s.beforeCachePut = hook
}

func NewEntitlementQueryService(store *repository.Store, cache *repository.ProjectionCache) *EntitlementQueryService {
	return &EntitlementQueryService{store: store, cache: cache}
}

// Detail returns the material-detail projection for a tenant and scope. On a cache
// miss it reads from the store together with the store version at read time, then
// writes the value back to the cache only when no PlanChange committed a newer
// version in the meantime. This keeps an in-flight Detail that read a now-stale
// value from repopulating the cache after a plan change.
func (s *EntitlementQueryService) Detail(ctx context.Context, tenant, scope string) (domain.Entity, error) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, err
	}
	if value, ok := s.cache.Get(ctx, tenant, scope); ok {
		return value, nil
	}
	value, readVersion, ok := s.store.FindWithVersion(ctx, tenant+"/"+scope)
	if !ok {
		if err := ctx.Err(); err != nil {
			return domain.Entity{}, err
		}
		return domain.Entity{}, ErrEntitlementNotFound
	}
	if s.beforeCachePut != nil {
		s.beforeCachePut()
	}
	if _, err := s.cache.PutIfCurrent(ctx, tenant, scope, value, readVersion); err != nil {
		return domain.Entity{}, err
	}
	return value, nil
}
