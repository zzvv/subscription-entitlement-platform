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

// CommitWithReceipt makes a subscription state change and its notification
// receipt visible together. It is idempotent per subscription key: the first
// confirmation for a tenant/scope commits the state and its single receipt,
// while any later confirmation for the same key returns the committed state
// and receipt unchanged instead of overwriting the state or appending a
// duplicate notification. This keeps exactly one state and one receipt per
// subscription even under concurrent confirmations.
func (s *Store) CommitWithReceipt(ctx context.Context, key string, value domain.Entity, receipt domain.Receipt) (domain.Entity, domain.Receipt, error) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, domain.Receipt{}, err
	}
	if s.commitBeforeLock != nil {
		s.commitBeforeLock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, domain.Receipt{}, err
	}
	// A receipt for this subscription already exists, so this is a duplicate
	// confirmation. Reuse the committed state and receipt rather than writing
	// a second notification or overwriting the survivor state.
	if ownerID, ok := s.receiptsByValue[key]; ok {
		if owner, ok := s.receipts[ownerID]; ok {
			return s.values[key], owner, nil
		}
	}
	if s.receiptCommitErr != nil {
		err := s.receiptCommitErr
		s.receiptCommitErr = nil
		return domain.Entity{}, domain.Receipt{}, err
	}
	s.values[key] = value
	s.receipts[receipt.ID] = receipt
	s.receiptsByValue[key] = receipt.ID
	return value, receipt, nil
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
