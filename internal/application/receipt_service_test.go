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

func TestConfirmKeepsTenantIsolationAcrossReceiptFailure(t *testing.T) {
	store := repository.NewStore()
	store.SetReceiptCommitErrorForTest(errors.New("notification outbox unavailable"))
	service := NewReceiptService(store)

	// A failed confirmation in tenant-a must not surface in any other tenant's
	// key space, and the subscription id must not be addressable via its own
	// tenant key either.
	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", "tenant-a", "standard", "activate")); err == nil {
		t.Fatal("expected receipt commit failure for tenant-a")
	}
	for _, key := range []string{"tenant-a/standard", "tenant-b/standard", "tenant-a/sub-a"} {
		if _, ok := store.Find(context.Background(), key); ok {
			t.Fatalf("failed confirmation leaked into %s", key)
		}
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-a/activate"); ok {
		t.Fatal("failed confirmation must not leave a notification receipt")
	}
}

func TestConfirmIsolatesReceiptsAndStateByTenant(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)

	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", "tenant-a", "standard", "activate")); err != nil {
		t.Fatalf("confirm tenant-a: %v", err)
	}
	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-b", "tenant-b", "standard", "activate")); err != nil {
		t.Fatalf("confirm tenant-b: %v", err)
	}

	// Each tenant only sees its own subscription key; the same scope value does
	// not collapse across tenants.
	if a, ok := store.Find(context.Background(), "tenant-a/standard"); !ok || a.Tenant != "tenant-a" || a.ID != "sub-a" {
		t.Fatalf("tenant-a state missing or wrong: %+v found=%t", a, ok)
	}
	if b, ok := store.Find(context.Background(), "tenant-b/standard"); !ok || b.Tenant != "tenant-b" || b.ID != "sub-b" {
		t.Fatalf("tenant-b state missing or wrong: %+v found=%t", b, ok)
	}

	// Receipts are keyed by subscription+action, so distinct subscriptions keep
	// distinct receipts even when the action matches.
	if _, ok := store.FindReceipt(context.Background(), "sub-a/activate"); !ok {
		t.Fatal("tenant-a receipt missing")
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-b/activate"); !ok {
		t.Fatal("tenant-b receipt missing")
	}
}

// TestConfirmRecoversAfterOneReceiptFailure confirms that a failed receipt
// commit is one-shot: once the notification outbox recovers, a subsequent
// normal confirmation lands both the subscription state and the notification
// receipt together. This guards the invariant that "a normal confirmation
// still persists both sides" even right after a failure, and that the store
// is not wedged failing forever.
func TestConfirmRecoversAfterOneReceiptFailure(t *testing.T) {
	store := repository.NewStore()
	store.SetReceiptCommitErrorForTest(errors.New("notification outbox unavailable"))
	service := NewReceiptService(store)

	// First confirmation fails because the notification receipt cannot land.
	if _, err := service.Confirm(context.Background(), domain.NewCommand("sub-a", "tenant-a", "standard", "activate")); err == nil {
		t.Fatal("expected first confirmation to fail while outbox is down")
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("failed confirmation must not leave subscription state")
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-a/activate"); ok {
		t.Fatal("failed confirmation must not leave notification receipt")
	}

	// Outbox recovers; the injected fault must not keep failing subsequent
	// confirmations. A normal confirmation must persist both sides together.
	entity, err := service.Confirm(context.Background(), domain.NewCommand("sub-b", "tenant-b", "standard", "activate"))
	if err != nil {
		t.Fatalf("expected recovered confirmation to succeed, got %v", err)
	}
	if entity.ID != "sub-b" || entity.Tenant != "tenant-b" {
		t.Fatalf("unexpected confirmed entity: %+v", entity)
	}
	if got, ok := store.Find(context.Background(), "tenant-b/standard"); !ok || got.ID != "sub-b" {
		t.Fatalf("recovered confirmation must persist subscription state: %+v found=%t", got, ok)
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-b/activate"); !ok {
		t.Fatal("recovered confirmation must persist notification receipt")
	}

	// The earlier failed tenant must still show neither side, keeping the
	// state and receipt consistent and isolated across the failure boundary.
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("recovered confirmation must not retroactively create the failed tenant's state")
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-a/activate"); ok {
		t.Fatal("recovered confirmation must not retroactively create the failed tenant's receipt")
	}
}
