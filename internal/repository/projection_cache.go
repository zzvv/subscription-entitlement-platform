package repository

import (
	"context"
	"sync"

	"example.com/subscription-entitlement-platform/internal/domain"
)

// ProjectionCache keeps material-detail projections close to the query path.
// Entries are scoped by the same tenant and subscription scope used by Store.
//
// To survive the Detail/PlanChange race, the cache is version-aware. Each entry
// carries the store version of the value it holds, and a per-key high-water mark
// records the latest store version the cache has been told about. A Detail only
// writes a value back when its read version is at least the high-water mark, so a
// value read before a concurrent PlanChange committed can never displace a newer
// one. PlanChange advances the high-water mark after it persists and evicts any
// entry left behind at an older version.
type ProjectionCache struct {
	mu        sync.RWMutex
	values    map[string]domain.Entity
	versions  map[string]uint64
	highWater map[string]uint64
	deleteErr error
}

// SetDeleteErrorForTest makes the next cache invalidation fail.
func (c *ProjectionCache) SetDeleteErrorForTest(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteErr = err
}

func NewProjectionCache() *ProjectionCache {
	return &ProjectionCache{
		values:    make(map[string]domain.Entity),
		versions:  make(map[string]uint64),
		highWater: make(map[string]uint64),
	}
}

func cacheKey(tenant, scope string) string {
	return tenant + "/" + scope
}

func (c *ProjectionCache) Get(ctx context.Context, tenant, scope string) (domain.Entity, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.values[cacheKey(tenant, scope)]
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
	c.values[cacheKey(tenant, scope)] = value
	return nil
}

// PutIfCurrent writes value back only when readVersion is at least the entry's
// high-water mark, i.e. no PlanChange committed a newer version between the read
// and now. It returns true when the write happened and false when a concurrent
// commit made the value stale, in which case the stale value is dropped rather
// than polluting the cache.
func (c *ProjectionCache) PutIfCurrent(ctx context.Context, tenant, scope string, value domain.Entity, readVersion uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	key := cacheKey(tenant, scope)
	if c.highWater[key] > readVersion {
		return false, nil
	}
	c.values[key] = value
	c.versions[key] = readVersion
	return true, nil
}

// Delete is the pre-commit invalidation step: it drops the cached projection so
// a subsequent read misses the cache, and reports the backend failure used to
// guard the persist-then-invalidate invariant. It deliberately leaves the
// high-water mark untouched; AdvanceVersion updates it after the commit.
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
	key := cacheKey(tenant, scope)
	delete(c.values, key)
	delete(c.versions, key)
	return nil
}

// AdvanceVersion records that the store has committed version for the entry and
// evicts any cached projection left at an older version. This is the post-commit
// backstop: even a Detail that slipped its stale write back in between Delete and
// the commit gets retired here, so the cache can never hold a value older than
// the store's current state.
func (c *ProjectionCache) AdvanceVersion(ctx context.Context, tenant, scope string, version uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	key := cacheKey(tenant, scope)
	if version > c.highWater[key] {
		c.highWater[key] = version
	}
	if c.versions[key] < c.highWater[key] {
		delete(c.values, key)
		delete(c.versions, key)
	}
	return nil
}
