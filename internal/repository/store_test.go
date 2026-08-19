package repository

import (
	"context"
	"errors"
	"testing"

	"example.com/subscription-entitlement-platform/internal/domain"
)

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

func TestLoadOrStoreDoesNotPersistWhenContextCancelsWhileWaitingForLock(t *testing.T) {
	store := NewStore()
	entered := make(chan struct{})
	release := make(chan struct{})
	store.SetLoadOrStoreBeforeLockForTest(func() {
		close(entered)
		<-release
	})

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, err := store.LoadOrStore(ctx, "tenant-a/standard", domain.Entity{ID: "sub-a"})
		result <- err
	}()

	<-entered
	cancel()
	close(release)
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled load or store, got %v", err)
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("canceled request must not leave subscription state")
	}
}
