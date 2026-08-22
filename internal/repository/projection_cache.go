package repository

import (
	"context"
	"sync"

	"example.com/subscription-entitlement-platform/internal/domain"
)

// ProjectionCache keeps material-detail projections close to the query path.
// Entries are scoped by the same tenant and subscription scope used by Store.
type ProjectionCache struct {
	mu     sync.RWMutex
	values map[string]domain.Entity
}

func NewProjectionCache() *ProjectionCache {
	return &ProjectionCache{values: make(map[string]domain.Entity)}
}

func (c *ProjectionCache) Get(ctx context.Context, tenant, scope string) (domain.Entity, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.values[c.key(tenant, scope)]
	return value, ok
}

func (c *ProjectionCache) Put(ctx context.Context, tenant, scope string, value domain.Entity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	c.values[c.key(tenant, scope)] = value
	return nil
}

// key namespaces cache entries by both tenant and subscription scope so that
// projections belonging to different tenants never collide, even when they
// share the same scope (e.g. both on a "standard" plan).
func (c *ProjectionCache) key(tenant, scope string) string {
	return tenant + "/" + scope
}
