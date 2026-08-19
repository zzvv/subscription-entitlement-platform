package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/subscription-entitlement-platform/internal/domain"
)

type cancelAfterFirstCheck struct{ checks int }

func (c *cancelAfterFirstCheck) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterFirstCheck) Done() <-chan struct{}       { return nil }
func (c *cancelAfterFirstCheck) Err() error {
	c.checks++
	if c.checks > 1 {
		return context.Canceled
	}
	return nil
}
func (c *cancelAfterFirstCheck) Value(any) any { return nil }

func TestStoreRejectsCanceledContext(t *testing.T) {
	store := NewStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Save(ctx, "tenant-a/standard", domain.Entity{ID: "sub-a"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled save, got %v", err)
	}
	if _, ok := store.Find(ctx, "tenant-a/standard"); ok {
		t.Fatal("canceled lookup must not return a value")
	}
}

func TestStoreDoesNotCommitAfterContextCancellation(t *testing.T) {
	store := NewStore()
	ctx := &cancelAfterFirstCheck{}
	value := domain.Entity{ID: "sub-a", Tenant: "tenant-a", Scope: "standard"}

	if err := store.Save(ctx, "tenant-a/standard", value); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation before commit, got %v", err)
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("canceled save must not commit subscription state")
	}
}

// TestStoreLoadOrStoreDoesNotCommitAfterContextCancellation 覆盖订阅命令处理流程
// 实际使用的写入入口 LoadOrStore：在取得写锁后再次发现 context 已取消时，
// 不得提交新的订阅状态，避免后续读取到本应取消的半截状态。
func TestStoreLoadOrStoreDoesNotCommitAfterContextCancellation(t *testing.T) {
	store := NewStore()
	ctx := &cancelAfterFirstCheck{}
	value := domain.Entity{ID: "sub-a", Tenant: "tenant-a", Scope: "standard"}

	if _, _, err := store.LoadOrStore(ctx, "tenant-a/standard", value); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation before commit, got %v", err)
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("canceled load or store must not commit subscription state")
	}
}

// TestStoreLoadOrStoreSkipsCommitWhileWaitingForWriteLock 模拟请求在等待写锁期间被取消：
// 另一个写入持有锁期间，被取消的 LoadOrStore 在取得锁后不得提交状态。
func TestStoreLoadOrStoreSkipsCommitWhileWaitingForWriteLock(t *testing.T) {
	store := NewStore()
	held := make(chan struct{})
	release := make(chan struct{})

	// 先让一个写入持有写锁，直到收到释放信号。
	go func() {
		store.mu.Lock()
		close(held)
		<-release
		store.mu.Unlock()
	}()
	<-held

	// 被“取消”的请求：首次检查通过（仍在等待写锁），取得锁后第二次检查返回已取消。
	cancelCtx := &cancelAfterFirstCheck{}
	value := domain.Entity{ID: "sub-a", Tenant: "tenant-a", Scope: "standard"}

	// 释放写锁，使被取消的请求得以取得锁并执行第二次 ctx.Err() 检查。
	close(release)

	_, _, err := store.LoadOrStore(cancelCtx, "tenant-a/standard", value)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation before commit, got %v", err)
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("canceled load or store must not commit subscription state while waiting for write lock")
	}
}

func TestStoreLoadOrStoreIsAtomic(t *testing.T) {
	store := NewStore()
	const workers = 32
	results := make(chan domain.Entity, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func(i int) {
			<-start
			value, _, err := store.LoadOrStore(context.Background(), "tenant-a/standard", domain.Entity{ID: string(rune('a' + i))})
			if err != nil {
				t.Errorf("load or store failed: %v", err)
				return
			}
			results <- value
		}(i)
	}
	close(start)
	first := <-results
	for i := 1; i < workers; i++ {
		if got := <-results; got != first {
			t.Fatalf("atomic load or store returned different values: first=%+v got=%+v", first, got)
		}
	}
}
