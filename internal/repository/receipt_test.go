package repository

import (
	"context"
	"errors"
	"sync"
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

func TestCommitWithReceiptExposesStateAndReceiptTogether(t *testing.T) {
	store := NewStore()
	entity := domain.NewEntity("sub-a", "tenant-a", "standard")
	receipt := domain.NewReceipt("sub-a/activate", "tenant-a", "sub-a", "activate")

	if err := store.CommitWithReceipt(context.Background(), "tenant-a/standard", entity, receipt); err != nil {
		t.Fatalf("commit with receipt: %v", err)
	}
	// A successful commit keeps both sides visible, mirroring the failure path
	// which keeps neither: there is no state-without-receipt window.
	got, ok := store.Find(context.Background(), "tenant-a/standard")
	if !ok {
		t.Fatal("successful commit must expose subscription state")
	}
	if got.ID != "sub-a" || got.Tenant != "tenant-a" {
		t.Fatalf("unexpected committed entity: %+v", got)
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-a/activate"); !ok {
		t.Fatal("successful commit must expose notification receipt")
	}
}

// TestCommitWithReceiptClearsFaultAfterSurfacing asserts that the injected
// receipt fault is one-shot: after a commit fails, the fault is cleared so a
// following commit on the same store recovers and lands both sides together.
// This keeps the in-memory adapter usable after a transient notification
// outage instead of failing every commit thereafter.
func TestCommitWithReceiptClearsFaultAfterSurfacing(t *testing.T) {
	store := NewStore()
	store.SetReceiptCommitErrorForTest(errors.New("notification outbox unavailable"))
	entity := domain.NewEntity("sub-a", "tenant-a", "standard")
	receipt := domain.NewReceipt("sub-a/activate", "tenant-a", "sub-a", "activate")

	if err := store.CommitWithReceipt(context.Background(), "tenant-a/standard", entity, receipt); err == nil {
		t.Fatal("expected first commit to fail while fault is set")
	}
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("failed commit must not expose subscription state")
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-a/activate"); ok {
		t.Fatal("failed commit must not expose notification receipt")
	}

	// No further fault injection; the store must recover.
	recovered := domain.NewEntity("sub-b", "tenant-b", "standard")
	recoveredReceipt := domain.NewReceipt("sub-b/activate", "tenant-b", "sub-b", "activate")
	if err := store.CommitWithReceipt(context.Background(), "tenant-b/standard", recovered, recoveredReceipt); err != nil {
		t.Fatalf("expected recovered commit to succeed, got %v", err)
	}
	if _, ok := store.Find(context.Background(), "tenant-b/standard"); !ok {
		t.Fatal("recovered commit must expose subscription state")
	}
	if _, ok := store.FindReceipt(context.Background(), "sub-b/activate"); !ok {
		t.Fatal("recovered commit must expose notification receipt")
	}
}

// TestCommitWithReceiptKeepsStateAndReceiptConsistentConcurrently asserts that
// under concurrent commits the subscription state and its notification receipt
// are always observable together: every committed receipt has a matching
// subscription key, so there is never a confirmed subscription whose
// notification record went missing mid-commit.
func TestCommitWithReceiptKeepsStateAndReceiptConsistentConcurrently(t *testing.T) {
	store := NewStore()
	const workers = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		id := "sub-" + itoa(i)
		tenant := "tenant-" + string(rune('a'+i%4))
		go func() {
			defer wg.Done()
			<-start
			entity := domain.NewEntity(id, tenant, "standard")
			receipt := domain.NewReceipt(id+"/activate", tenant, id, "activate")
			if err := store.CommitWithReceipt(context.Background(), tenant+"/standard", entity, receipt); err != nil {
				t.Errorf("concurrent commit failed: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	// Each committed receipt must resolve to a subscription key that holds an
	// entity from the same tenant. Because state and receipt are written under
	// a single lock, a receipt can never exist without its matching state.
	store.mu.RLock()
	if len(store.receipts) != workers {
		store.mu.RUnlock()
		t.Fatalf("expected %d receipts, got %d", workers, len(store.receipts))
	}
	for _, receipt := range store.receipts {
		key := receipt.Tenant + "/standard"
		entity, ok := store.values[key]
		if !ok {
			store.mu.RUnlock()
			t.Fatalf("receipt %s has no matching subscription state in %s", receipt.ID, key)
		}
		if entity.Tenant != receipt.Tenant {
			store.mu.RUnlock()
			t.Fatalf("receipt %s (tenant %s) points at state for tenant %s", receipt.ID, receipt.Tenant, entity.Tenant)
		}
	}
	store.mu.RUnlock()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
