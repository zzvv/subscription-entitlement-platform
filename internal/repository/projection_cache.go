package repository

import (
	"context"
	"sync"

	"example.com/subscription-entitlement-platform/internal/domain"
)

// ProjectionCache keeps material-detail projections close to the query path.
// Entries are scoped by the same tenant and subscription scope used by Store.
type ProjectionCache struct {
	mu            sync.RWMutex
	values        map[string]domain.Entity
	deleteErr     error
	getBeforeLock func()
}

// SetGetBeforeLockForTest pauses Get after its initial context check, before
// it acquires the read lock. Used to reproduce cancellation while waiting.
func (c *ProjectionCache) SetGetBeforeLockForTest(hook func()) {
	c.getBeforeLock = hook
}

// SetDeleteErrorForTest makes the next cache invalidation fail.
func (c *ProjectionCache) SetDeleteErrorForTest(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteErr = err
}

func NewProjectionCache() *ProjectionCache {
	return &ProjectionCache{values: make(map[string]domain.Entity)}
}

func (c *ProjectionCache) Get(ctx context.Context, tenant, scope string) (domain.Entity, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false
	}
	if c.getBeforeLock != nil {
		c.getBeforeLock()
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	// A detail query canceled while waiting for the cache read lock must not
	// return a cached subscription projection, so treat it as a miss.
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false
	}
	value, ok := c.values[tenant+"/"+scope]
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
	c.values[tenant+"/"+scope] = value
	return nil
}

// Delete removes the material-detail projection after its subscription state changes.
func (c *ProjectionCache) Delete(ctx context.Context, tenant, scope string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.deleteErr != nil {
		err := c.deleteErr
		c.deleteErr = nil
		return err
	}
	delete(c.values, tenant+"/"+scope)
	return nil
}
