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

// TestConcurrentConfirmationsAreIdempotentPerSubscription drives many workers
// confirming the same tenant/scope with different command ids at once and
// asserts that exactly one state and one notification survive, run after run,
// without relying on the before-lock test hook. It is the stable regression
// for the duplicate-notification / state-overwrite concurrency bug.
func TestConcurrentConfirmationsAreIdempotentPerSubscription(t *testing.T) {
	const workers = 64
	const rounds = 50
	for round := 0; round < rounds; round++ {
		store := repository.NewStore()
		service := NewReceiptService(store)

		start := make(chan struct{})
		errs := make(chan error, workers)
		for i := 0; i < workers; i++ {
			command := domain.NewCommand("sub-"+itoa(i), "tenant-a", "standard", "activate")
			go func() {
				<-start
				_, err := service.Confirm(context.Background(), command)
				errs <- err
			}()
		}
		close(start)
		for i := 0; i < workers; i++ {
			if err := <-errs; err != nil {
				t.Fatalf("round %d: confirmation failed: %v", round, err)
			}
		}

		entity, ok := store.Find(context.Background(), "tenant-a/standard")
		if !ok {
			t.Fatalf("round %d: concurrent confirmation must leave subscription state", round)
		}
		if entity.Tenant != "tenant-a" || entity.Scope != "standard" {
			t.Fatalf("round %d: survivor state leaked across scope: %+v", round, entity)
		}
		if got := store.ReceiptCountForTest(); got != 1 {
			t.Fatalf("round %d: expected one notification receipt, got %d", round, got)
		}
	}
}

// TestConcurrentConfirmationsKeepTenantsIsolated confirms that the idempotent
// commit is scoped per tenant/scope, so concurrent confirmations in different
// tenants never cross wires.
func TestConcurrentConfirmationsKeepTenantsIsolated(t *testing.T) {
	store := repository.NewStore()
	service := NewReceiptService(store)

	tenants := []string{"tenant-a", "tenant-b", "tenant-c"}
	const perTenant = 24
	start := make(chan struct{})
	errs := make(chan error, len(tenants)*perTenant)
	for _, tenant := range tenants {
		for i := 0; i < perTenant; i++ {
			tenant := tenant
			go func() {
				<-start
				_, err := service.Confirm(context.Background(),
					domain.NewCommand("sub-"+tenant+"-"+itoa(i), tenant, "standard", "activate"))
				errs <- err
			}()
		}
	}
	close(start)
	for i := 0; i < len(tenants)*perTenant; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("confirmation failed: %v", err)
		}
	}

	for _, tenant := range tenants {
		entity, ok := store.Find(context.Background(), tenant+"/standard")
		if !ok {
			t.Fatalf("tenant %s: expected committed subscription state", tenant)
		}
		if entity.Tenant != tenant {
			t.Fatalf("tenant %s received another tenant's state: %+v", tenant, entity)
		}
	}
	if got := store.ReceiptCountForTest(); got != len(tenants) {
		t.Fatalf("expected one receipt per tenant (%d), got %d", len(tenants), got)
	}
}

// itoa returns the decimal representation of n. It avoids pulling in strconv
// only to keep the test file self-contained.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
