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
