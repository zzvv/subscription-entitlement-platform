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

func TestConcurrentConfirmationsReturnSameSubscriptionState(t *testing.T) {
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
	type outcome struct {
		entity domain.Entity
		err    error
	}
	results := make(chan outcome, len(commands))
	for _, command := range commands {
		go func(command domain.Command) {
			entity, err := service.Confirm(context.Background(), command)
			results <- outcome{entity: entity, err: err}
		}(command)
	}
	<-entered
	<-entered
	close(release)

	outcomes := make([]outcome, 0, len(commands))
	for range commands {
		got := <-results
		if got.err != nil {
			t.Fatalf("concurrent confirmation failed: %v", got.err)
		}
		outcomes = append(outcomes, got)
	}
	// Concurrent confirmations of the same tenant/scope must collapse onto a single
	// committed subscription state, regardless of which operator's command landed first.
	authoritative, ok := store.Find(context.Background(), "tenant-a/standard")
	if !ok {
		t.Fatal("concurrent confirmation must leave subscription state")
	}
	for i, got := range outcomes {
		if got.entity != authoritative {
			t.Fatalf("operator %d returned %+v, expected the single committed state %+v", i, got.entity, authoritative)
		}
	}
	if got := store.ReceiptCountForTest(); got != 1 {
		t.Fatalf("expected one notification receipt for one subscription, got %d", got)
	}
}

func TestRepeatedConfirmationIsIdempotent(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)
	command := domain.NewCommand("sub-a", "tenant-a", "standard", "activate")

	first, err := service.Confirm(context.Background(), command)
	if err != nil {
		t.Fatalf("first confirmation: %v", err)
	}
	second, err := service.Confirm(context.Background(), command)
	if err != nil {
		t.Fatalf("idempotent reconfirmation: %v", err)
	}
	if second != first {
		t.Fatalf("reconfirmation must return the same committed state: first=%+v second=%+v", first, second)
	}
	if got := store.ReceiptCountForTest(); got != 1 {
		t.Fatalf("idempotent reconfirmation must not enqueue another receipt, got %d", got)
	}
}

func TestConfirmationIdempotencyDoesNotBleedAcrossTenantsOrScopes(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)

	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", "tenant-a", "standard", "activate")); err != nil {
		t.Fatalf("confirm tenant-a/standard: %v", err)
	}
	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-b", "tenant-b", "standard", "activate")); err != nil {
		t.Fatalf("confirm tenant-b/standard: %v", err)
	}
	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-c", "tenant-a", "premium", "activate")); err != nil {
		t.Fatalf("confirm tenant-a/premium: %v", err)
	}

	tenantAStandard, ok := store.Find(context.Background(), "tenant-a/standard")
	if !ok || tenantAStandard.ID != "sub-a" {
		t.Fatalf("tenant-a/standard must keep its own state: %+v ok=%t", tenantAStandard, ok)
	}
	tenantBStandard, ok := store.Find(context.Background(), "tenant-b/standard")
	if !ok || tenantBStandard.ID != "sub-b" {
		t.Fatalf("tenant-b/standard must keep its own state: %+v ok=%t", tenantBStandard, ok)
	}
	tenantAPremium, ok := store.Find(context.Background(), "tenant-a/premium")
	if !ok || tenantAPremium.ID != "sub-c" {
		t.Fatalf("tenant-a/premium must keep its own state: %+v ok=%t", tenantAPremium, ok)
	}
	if got := store.ReceiptCountForTest(); got != 3 {
		t.Fatalf("isolated confirmations must each enqueue their own receipt, got %d", got)
	}
}
