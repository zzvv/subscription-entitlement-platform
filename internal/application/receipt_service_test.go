package application

import (
	"context"
	"errors"
	"testing"

	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
)

func TestConfirmCommitsSubscriptionAndReceiptTogether(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)

	entity, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", "tenant-a", "standard", "activate"))
	if err != nil {
		t.Fatalf("confirm subscription: %v", err)
	}
	if entity.ID != "sub-a" {
		t.Fatalf("unexpected subscription: %+v", entity)
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); !ok {
		t.Fatal("confirmed subscription state was not committed")
	}
	if receipt, ok := store.FindReceipt(context.Background(), "sub-a/activate"); !ok || receipt.Status != "pending" {
		t.Fatalf("notification receipt was not committed: %+v, found=%t", receipt, ok)
	}
}

func TestConfirmAddsReceiptWhenSubscriptionAlreadyExists(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)
	if err := store.Save(context.Background(), "tenant-a/standard", domain.NewEntity("sub-a", "tenant-a", "standard")); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", "tenant-a", "standard", "activate")); err != nil {
		t.Fatalf("confirm existing subscription: %v", err)
	}
	if receipt, ok := store.FindReceipt(context.Background(), "sub-a/activate"); !ok || receipt.Status != "pending" {
		t.Fatalf("existing subscription confirmation lost its receipt: %+v found=%t", receipt, ok)
	}
}

func TestConfirmLeavesNoPartialStateWhenReceiptCommitFails(t *testing.T) {
	store := repository.NewStore()
	store.SetReceiptCommitErrorForTest(errors.New("notification outbox unavailable"))
	service := NewReceiptService(store)

	_, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", "tenant-a", "standard", "activate"))
	if err == nil {
		t.Fatal("expected receipt commit failure")
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("failed confirmation must not leave subscription state")
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-a/activate"); ok {
		t.Fatal("failed confirmation must not leave notification receipt")
	}
}

func TestTransientReceiptFailureDoesNotPoisonNextConfirmation(t *testing.T) {
	store := repository.NewStore()
	store.SetReceiptCommitErrorForTest(errors.New("temporary receipt outage"))
	service := NewReceiptService(store)
	command := domain.NewCommand("sub-a", "tenant-a", "standard", "activate")

	if _, err := service.Confirm(context.Background(), command); err == nil {
		t.Fatal("expected first confirmation to fail")
	}
	if _, err := service.Confirm(context.Background(), command); err != nil {
		t.Fatalf("retry should succeed after transient receipt failure: %v", err)
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); !ok {
		t.Fatal("successful retry must persist subscription state")
	}
}

func TestConfirmDoesNotDuplicateReceiptForAlreadyConfirmedSubscription(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)
	command := domain.NewCommand("sub-a", "tenant-a", "standard", "activate")

	if _, err := service.Confirm(context.Background(), command); err != nil {
		t.Fatalf("first confirmation: %v", err)
	}
	if _, err := service.Confirm(context.Background(), command); err != nil {
		t.Fatalf("second confirmation: %v", err)
	}
	if got := store.ReceiptCountForTest(); got != 1 {
		t.Fatalf("duplicate confirmation must not create an extra receipt, got %d", got)
	}
	if got, ok := store.Find(context.Background(), "tenant-a/standard"); !ok || got.ID != "sub-a" {
		t.Fatalf("subscription must be preserved unchanged: %+v found=%t", got, ok)
	}
}

func TestConcurrentConfirmationsKeepOneSubscriptionStateAndReceipt(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)
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
	results := make(chan error, len(commands))
	for _, command := range commands {
		go func(command domain.Command) {
			_, err := service.Confirm(context.Background(), command)
			results <- err
		}(command)
	}
	<-entered
	<-entered
	close(release)
	for range commands {
		if err := <-results; err != nil {
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
