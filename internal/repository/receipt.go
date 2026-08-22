package repository

import (
	"context"

	"example.com/subscription-entitlement-platform/internal/domain"
)

// SetReceiptCommitErrorForTest makes the next receipt commit fail in the in-memory adapter.
func (s *Store) SetReceiptCommitErrorForTest(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receiptCommitErr = err
}

// CommitWithReceipt makes a subscription state change and its notification receipt visible together.
// It keeps the existing per-tenant-and-scope idempotency: a subscription that is already saved is
// never overwritten, and a request that does not actually persist state must not produce an extra
// receipt. Each subscription maps to at most one notification receipt, even under concurrent
// confirmations carrying different command identifiers.
func (s *Store) CommitWithReceipt(ctx context.Context, key string, value domain.Entity, receipt domain.Receipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.commitBeforeLock != nil {
		s.commitBeforeLock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.receiptCommitErr != nil {
		err := s.receiptCommitErr
		s.receiptCommitErr = nil
		return err
	}
	if _, exists := s.values[key]; !exists {
		s.values[key] = value
	}
	if _, exists := s.receiptsByKey[key]; !exists {
		s.receipts[receipt.ID] = receipt
		s.receiptsByKey[key] = struct{}{}
	}
	return nil
}

// FindReceipt looks up a notification receipt by its identifier.
func (s *Store) FindReceipt(ctx context.Context, receiptID string) (domain.Receipt, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Receipt{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	receipt, ok := s.receipts[receiptID]
	return receipt, ok
}
