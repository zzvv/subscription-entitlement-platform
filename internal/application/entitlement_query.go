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
	store *repository.Store
	cache *repository.ProjectionCache
}

func NewEntitlementQueryService(store *repository.Store, cache *repository.ProjectionCache) *EntitlementQueryService {
	return &EntitlementQueryService{store: store, cache: cache}
}

func (s *EntitlementQueryService) Detail(ctx context.Context, tenant, scope string) (domain.Entity, error) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, err
	}
	if value, ok := s.cache.Get(ctx, tenant, scope); ok {
		return value, nil
	}
	value, ok := s.store.Find(ctx, tenant+"/"+scope)
	if !ok {
		if err := ctx.Err(); err != nil {
			return domain.Entity{}, err
		}
		return domain.Entity{}, ErrEntitlementNotFound
	}
	// The cache is an optimization for a successful store read. A transient
	// cache outage must not turn an available subscription into a failed read,
	// but request cancellation still belongs to the caller's lifecycle.
	if err := s.cache.Put(ctx, tenant, scope, value); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return domain.Entity{}, ctxErr
		}
	}
	return value, nil
}
