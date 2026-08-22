package repository

import (
	"context"
	"testing"

	"example.com/subscription-entitlement-platform/internal/domain"
)

// TestCacheGetDoesNotReturnAfterContextCancelsWhileWaitingForLock reproduces
// the boundary case where a detail query is canceled while waiting for the
// cache read lock. A cached subscription projection must not be surfaced for a
// canceled request, mirroring Store.Find.
func TestCacheGetDoesNotReturnAfterContextCancelsWhileWaitingForLock(t *testing.T) {
	cache := NewProjectionCache()
	if err := cache.Put(context.Background(), "tenant-a", "standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed cached projection: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	cache.SetGetBeforeLockForTest(func() {
		close(entered)
		<-release
	})

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan bool, 1)
	go func() {
		_, ok := cache.Get(ctx, "tenant-a", "standard")
		result <- ok
	}()

	<-entered
	cache.mu.Lock()
	cancel()
	close(release)
	cache.mu.Unlock()
	if <-result {
		t.Fatal("canceled cache lookup must not return a cached projection after waiting for the lock")
	}
}
