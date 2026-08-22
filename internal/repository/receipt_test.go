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

// A subscription saved through any path that did not schedule its notification must still
// receive a receipt the first time an operator confirms it, so the page no longer looks
// complete while notifications silently never fire.
func TestCommitWithReceiptLeavesReceiptWhenSubscriptionAlreadyExists(t *testing.T) {
	store := NewStore()
	if err := store.Save(context.Background(), "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	entity := domain.NewEntity("sub-a", "tenant-a", "standard")
	receipt := domain.NewReceipt("sub-a/activate", "tenant-a", "sub-a", "activate")

	if err := store.CommitWithReceipt(context.Background(), "tenant-a/standard", entity, receipt); err != nil {
		t.Fatalf("confirm existing subscription: %v", err)
	}
	got, ok := store.Find(context.Background(), "tenant-a/standard")
	if !ok {
		t.Fatal("existing subscription state was dropped")
	}
	if got.ID != "sub-a" {
		t.Fatalf("existing subscription was overwritten: %+v", got)
	}
	if receipt, ok := store.FindReceipt(context.Background(), "sub-a/activate"); !ok || receipt.Status != "pending" {
		t.Fatalf("confirmation of an existing subscription lost its receipt: %+v found=%t", receipt, ok)
	}
}

// Re-confirming an already-confirmed subscription must be idempotent: the saved subscription is
// preserved and no second receipt is produced for a request that did not really persist state.
func TestCommitWithReceiptDoesNotDuplicateReceiptForAlreadyConfirmedSubscription(t *testing.T) {
	store := NewStore()
	entity := domain.NewEntity("sub-a", "tenant-a", "standard")
	receipt := domain.NewReceipt("sub-a/activate", "tenant-a", "sub-a", "activate")
	key := "tenant-a/standard"

	if err := store.CommitWithReceipt(context.Background(), key, entity, receipt); err != nil {
		t.Fatalf("first confirmation: %v", err)
	}
	if err := store.CommitWithReceipt(context.Background(), key, entity, receipt); err != nil {
		t.Fatalf("second confirmation: %v", err)
	}
	if got := store.ReceiptCountForTest(); got != 1 {
		t.Fatalf("duplicate confirmation must not create extra receipts, got %d", got)
	}
	if got, ok := store.Find(context.Background(), key); !ok || got.ID != "sub-a" {
		t.Fatalf("subscription must be preserved unchanged: %+v found=%t", got, ok)
	}
}

// Under concurrent confirmations for the same tenant and scope (even with different command
// identifiers), exactly one subscription state and one notification receipt must survive.
func TestCommitWithReceiptConcurrentConfirmsKeepOneReceipt(t *testing.T) {
	store := NewStore()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	store.SetCommitBeforeLockForTest(func() {
		entered <- struct{}{}
		<-release
	})

	commands := []domain.Command{
		domain.NewCommand("sub-a", "tenant-a", "standard", "activate"),
		domain.NewCommand("sub-b", "tenant-a", "standard", "activate"),
	}
	type commit struct {
		key     string
		value   domain.Entity
		receipt domain.Receipt
	}
	commits := make([]commit, 0, len(commands))
	for _, command := range commands {
		key := command.Tenant + "/" + command.Scope
		value := domain.NewEntity(command.ID, command.Tenant, command.Scope)
		value.Plan = command.Plan
		receipt := domain.NewReceipt(command.ID+"/activate", command.Tenant, command.ID, command.Action)
		commits = append(commits, commit{key: key, value: value, receipt: receipt})
	}

	errs := make(chan error, len(commits))
	for _, c := range commits {
		go func(c commit) {
			errs <- store.CommitWithReceipt(context.Background(), c.key, c.value, c.receipt)
		}(c)
	}
	<-entered
	<-entered
	close(release)
	for range commits {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent confirmation failed: %v", err)
		}
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); !ok {
		t.Fatal("concurrent confirmation must leave subscription state")
	}
	if got := store.ReceiptCountForTest(); got != 1 {
		t.Fatalf("expected one notification receipt for one subscription, got %d", got)
	}
}

// When two operators confirm a subscription that was already saved without a receipt at the same
// time, exactly one notification receipt survives. Without the fix both confirmations hit the
// already-exists branch and leave zero receipts, so notifications would never fire.
func TestCommitWithReceiptConcurrentConfirmsOnPreExistingSubscriptionKeepOneReceipt(t *testing.T) {
	store := NewStore()
	key := "tenant-a/standard"
	if err := store.Save(context.Background(), key, domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	store.SetCommitBeforeLockForTest(func() {
		entered <- struct{}{}
		<-release
	})

	ids := []string{"sub-a", "sub-b"}
	errs := make(chan error, len(ids))
	for _, id := range ids {
		value := domain.NewEntity(id, "tenant-a", "standard")
		receipt := domain.NewReceipt(id+"/activate", "tenant-a", id, "activate")
		go func(value domain.Entity, receipt domain.Receipt) {
			errs <- store.CommitWithReceipt(context.Background(), key, value, receipt)
		}(value, receipt)
	}
	<-entered
	<-entered
	close(release)
	for range ids {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent confirmation failed: %v", err)
		}
	}
	if got, ok := store.Find(context.Background(), key); !ok || got.ID != "sub-a" {
		t.Fatalf("pre-existing subscription must be preserved, got %+v found=%t", got, ok)
	}
	if got := store.ReceiptCountForTest(); got != 1 {
		t.Fatalf("expected one notification receipt for one subscription, got %d", got)
	}
}
