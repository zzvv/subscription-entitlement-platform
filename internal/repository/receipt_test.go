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

	if _, _, err := store.CommitWithReceipt(context.Background(), "tenant-a/standard", entity, receipt); err == nil {
		t.Fatal("expected receipt commit failure")
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("failed receipt commit must not expose subscription state")
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-a/activate"); ok {
		t.Fatal("failed receipt commit must not expose notification receipt")
	}
}

func TestCommitWithReceiptIsIdempotentForCommittedKey(t *testing.T) {
	store := NewStore()
	first := domain.NewEntity("sub-a", "tenant-a", "standard")
	firstReceipt := domain.NewReceipt("sub-a/activate", "tenant-a", "sub-a", "activate")

	stored, loaded, err := store.CommitWithReceipt(context.Background(), "tenant-a/standard", first, firstReceipt)
	if err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if loaded {
		t.Fatal("first commit of a fresh key must report loaded=false")
	}
	if stored != first {
		t.Fatalf("first commit must return the stored entity: got %+v", stored)
	}

	// A second operator confirms the same tenant/scope under a different subscription id.
	// The committed state and the single receipt must survive unchanged.
	duplicate := domain.NewEntity("sub-b", "tenant-a", "standard")
	duplicateReceipt := domain.NewReceipt("sub-b/activate", "tenant-a", "sub-b", "activate")
	storedAgain, loaded, err := store.CommitWithReceipt(context.Background(), "tenant-a/standard", duplicate, duplicateReceipt)
	if err != nil {
		t.Fatalf("idempotent commit: %v", err)
	}
	if !loaded {
		t.Fatal("idempotent commit of an existing key must report loaded=true")
	}
	if storedAgain != first {
		t.Fatalf("idempotent commit must return the existing state: got %+v want %+v", storedAgain, first)
	}
	if got := store.ReceiptCountForTest(); got != 1 {
		t.Fatalf("idempotent commit must not enqueue a second receipt, got %d", got)
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-b/activate"); ok {
		t.Fatal("idempotent commit must not store the duplicate receipt")
	}
}
