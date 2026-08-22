package repository

import (
	"context"

	"example.com/subscription-entitlement-platform/internal/domain"
)

// SetReceiptCommitErrorForTest makes the next receipt commit fail in the in-memory adapter,
// modelling a transient notification outage. The fault clears after it trips so a retry
// can succeed once the outage has passed.
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.receiptCommitErr != nil {
		// The fault hook models a transient notification outage: it must fail at most
		// the next commit and then recover, otherwise a single blip poisons every
		// retry on the same subscription and the confirmation can never complete.
		err := s.receiptCommitErr
		s.receiptCommitErr = nil
		return err
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
