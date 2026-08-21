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
	putErr        error
	putBeforeLock func()
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

// SetPutErrorForTest makes the next cache write fail in the in-memory adapter.
func (c *ProjectionCache) SetPutErrorForTest(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putErr = err
}

// SetPutBeforeLockForTest pauses Put after its initial context check.
func (c *ProjectionCache) SetPutBeforeLockForTest(hook func()) {
	c.putBeforeLock = hook
}

func (c *ProjectionCache) Get(ctx context.Context, tenant, scope string) (domain.Entity, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.values[tenant+"/"+scope]
	return value, ok
}

func (c *ProjectionCache) Put(ctx context.Context, tenant, scope string, value domain.Entity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.putBeforeLock != nil {
		c.putBeforeLock()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.putErr != nil {
		err := c.putErr
		c.putErr = nil
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
