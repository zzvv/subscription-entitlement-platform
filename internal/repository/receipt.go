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
// It is idempotent per subscription key: when the key already holds a committed state, the
// existing state and its receipt are returned unchanged and no second receipt is scheduled.
// This keeps concurrent confirmations of the same tenant/scope to one subscription state and
// one notification receipt, while normal confirmation flow and tenant isolation are unaffected.
func (s *Store) CommitWithReceipt(ctx context.Context, key string, value domain.Entity, receipt domain.Receipt) (domain.Entity, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false, err
	}
	if s.commitBeforeLock != nil {
		s.commitBeforeLock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false, err
	}
	if s.receiptCommitErr != nil {
		err := s.receiptCommitErr
		s.receiptCommitErr = nil
		return domain.Entity{}, false, err
	}
	if existing, ok := s.values[key]; ok {
		return existing, true, nil
	}
	s.values[key] = value
	s.receipts[receipt.ID] = receipt
	return value, false, nil
}

func (s *Store) FindReceipt(ctx context.Context, receiptID string) (domain.Receipt, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Receipt{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	receipt, ok := s.receipts[receiptID]
	return receipt, ok
}
