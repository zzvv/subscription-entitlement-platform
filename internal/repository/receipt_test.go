package repository

import (
	"context"
	"errors"
	"testing"

	"example.com/subscription-entitlement-platform/internal/domain"
)

func TestCommitWithReceiptDoesNotExposeStateWhenReceiptFails(t *testing.T) {
	store := NewStore()
	store.SetReceiptCommitErrorForTest(errors.New("notification outbox unavailable"))
	entity := domain.NewEntity("sub-a", "tenant-a", "standard")
	receipt := domain.NewReceipt("sub-a/activate", "tenant-a", "sub-a", "activate")

	if err := store.CommitWithReceipt(context.Background(), "tenant-a/standard", entity, receipt); err == nil {
		t.Fatal("expected receipt commit failure")
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("failed receipt commit must not expose subscription state")
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-a/activate"); ok {
		t.Fatal("failed receipt commit must not expose notification receipt")
	}
}

func TestCommitWithReceiptRecoversAfterTransientReceiptFailure(t *testing.T) {
	store := NewStore()
	store.SetReceiptCommitErrorForTest(errors.New("notification outbox unavailable"))
	entity := domain.NewEntity("sub-a", "tenant-a", "standard")
	receipt := domain.NewReceipt("sub-a/activate", "tenant-a", "sub-a", "activate")
	ctx := context.Background()

	// First attempt fails while the notification outage is in effect. The failure
	// must leave neither the half-written subscription state nor its receipt behind.
	if err := store.CommitWithReceipt(ctx, "tenant-a/standard", entity, receipt); err == nil {
		t.Fatal("expected transient receipt commit failure")
	}
	if _, ok := store.Find(ctx, "tenant-a/standard"); ok {
		t.Fatal("failed commit must not leave subscription state")
	}
	if _, ok := store.FindReceipt(ctx, "sub-a/activate"); ok {
		t.Fatal("failed commit must not leave notification receipt")
	}

	// The outage was transient. Retrying the same confirmation must now succeed and
	// persist both the subscription state and its receipt, otherwise the page would
	// stay stuck in the pending state forever.
	if err := store.CommitWithReceipt(ctx, "tenant-a/standard", entity, receipt); err != nil {
		t.Fatalf("retry should succeed after transient receipt failure: %v", err)
	}
	stored, ok := store.Find(ctx, "tenant-a/standard")
	if !ok {
		t.Fatal("successful retry must persist subscription state")
	}
	if stored.ID != "sub-a" {
		t.Fatalf("unexpected subscription state: %+v", stored)
	}
	gotReceipt, ok := store.FindReceipt(ctx, "sub-a/activate")
	if !ok {
		t.Fatal("successful retry must persist notification receipt")
	}
	if gotReceipt.Status != "pending" {
		t.Fatalf("notification receipt must remain pending after retry: %+v", gotReceipt)
	}
}
