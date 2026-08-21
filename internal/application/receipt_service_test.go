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
	if receipt, ok := store.FindReceipt(context.Background(), "tenant-a/sub-a/activate"); !ok || receipt.Status != "pending" {
		t.Fatalf("notification receipt was not committed: %+v, found=%t", receipt, ok)
	}
	if receipt, ok := store.FindReceipt(context.Background(), "tenant-a/sub-a/activate"); !ok || receipt.Tenant != "tenant-a" {
		t.Fatalf("notification receipt belongs to wrong tenant: %+v, found=%t", receipt, ok)
	}
}

func TestConfirmKeepsReceiptsIsolatedWhenTenantsReuseSubscriptionID(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", tenant, "standard", "activate")); err != nil {
			t.Fatalf("confirm %s: %v", tenant, err)
		}
	}
	if got := store.ReceiptCountForTest(); got != 2 {
		t.Fatalf("same subscription ID in different tenants lost a receipt: got %d", got)
	}
	// Each tenant keeps its own receipt and can read it back without picking up
	// the other tenant's record.
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		receipt, ok := store.FindReceipt(context.Background(), tenant+"/sub-a/activate")
		if !ok {
			t.Fatalf("tenant %s receipt missing", tenant)
		}
		if receipt.Tenant != tenant {
			t.Fatalf("tenant %s receipt belonged to %s", tenant, receipt.Tenant)
		}
		if receipt.SubscriptionID != "sub-a" {
			t.Fatalf("tenant %s receipt had unexpected subscription %q", tenant, receipt.SubscriptionID)
		}
	}
}

func TestConfirmDoesNotLeakReceiptsAcrossTenantsBySubscriptionID(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)

	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", "tenant-a", "standard", "activate")); err != nil {
		t.Fatalf("confirm tenant-a: %v", err)
	}
	// tenant-b reuses the same subscription id; it must not overwrite tenant-a's
	// receipt and tenant-a's id must not resolve to tenant-b's record.
	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", "tenant-b", "standard", "activate")); err != nil {
		t.Fatalf("confirm tenant-b: %v", err)
	}

	a, ok := store.FindReceipt(context.Background(), "tenant-a/sub-a/activate")
	if !ok || a.Tenant != "tenant-a" {
		t.Fatalf("tenant-a receipt was overwritten or missing: %+v found=%t", a, ok)
	}
	b, ok := store.FindReceipt(context.Background(), "tenant-b/sub-a/activate")
	if !ok || b.Tenant != "tenant-b" {
		t.Fatalf("tenant-b receipt was overwritten or missing: %+v found=%t", b, ok)
	}
	// The two receipts are independent records, not aliases of each other.
	if a.ID == b.ID {
		t.Fatalf("tenants shared one receipt id: %s", a.ID)
	}
}

func TestConfirmStaysIdempotentWithinATenant(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)
	command := domain.NewCommand("sub-a", "tenant-a", "standard", "activate")

	for i := 0; i < 3; i++ {
		if _, err := service.Confirm(context.Background(), command); err != nil {
			t.Fatalf("confirm attempt %d: %v", i+1, err)
		}
	}
	if got := store.ReceiptCountForTest(); got != 1 {
		t.Fatalf("repeated confirmation within one tenant must keep a single receipt: got %d", got)
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); !ok {
		t.Fatal("repeated confirmation must keep the subscription state")
	}
	receipt, ok := store.FindReceipt(context.Background(), "tenant-a/sub-a/activate")
	if !ok || receipt.Tenant != "tenant-a" {
		t.Fatalf("idempotent confirmation must keep the tenant-scoped receipt: %+v found=%t", receipt, ok)
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
	if _, ok := store.FindReceipt(context.Background(), "tenant-a/sub-a/activate"); ok {
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
