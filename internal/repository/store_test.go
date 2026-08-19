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
