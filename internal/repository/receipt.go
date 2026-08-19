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
	if _, exists := s.values[key]; exists {
		return nil
	}
	s.values[key] = value
	s.receipts[receipt.ID] = receipt
	return nil
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
