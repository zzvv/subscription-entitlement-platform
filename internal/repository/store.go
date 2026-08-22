package repository

import (
	"context"
	"example.com/subscription-entitlement-platform/internal/domain"
	"sync"
)

type Store struct {
	mu                    sync.RWMutex
	values                map[string]domain.Entity
	receipts              map[string]domain.Receipt
	receiptCommitErr      error
	findBeforeLock        func()
	loadOrStoreBeforeLock func()
	commitBeforeLock      func()
}

func NewStore() *Store {
	return &Store{
		values:   map[string]domain.Entity{},
		receipts: map[string]domain.Receipt{},
	}
}
func (s *Store) Find(ctx context.Context, key string) (domain.Entity, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false
	}
	if s.findBeforeLock != nil {
		s.findBeforeLock()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	// The request may have been canceled while waiting for the read lock. A
	// canceled detail query must not surface the subscription entity, so treat
	// the value as absent once ownership of the lock is acquired.
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false
	}
	value, ok := s.values[key]
	return value, ok
}
func (s *Store) Save(ctx context.Context, key string, value domain.Entity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

// SetLoadOrStoreBeforeLockForTest pauses LoadOrStore after its initial context check.
func (s *Store) SetLoadOrStoreBeforeLockForTest(hook func()) {
	s.loadOrStoreBeforeLock = hook
}

// SetFindBeforeLockForTest pauses Find after its initial context check.
func (s *Store) SetFindBeforeLockForTest(hook func()) {
	s.findBeforeLock = hook
}

// SetCommitBeforeLockForTest pauses CommitWithReceipt before it acquires the store lock.
func (s *Store) SetCommitBeforeLockForTest(hook func()) {
	s.commitBeforeLock = hook
}

func (s *Store) ReceiptCountForTest() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.receipts)
}

// LoadOrStore atomically returns the existing value or stores value when key is absent.
func (s *Store) LoadOrStore(ctx context.Context, key string, value domain.Entity) (domain.Entity, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false, err
	}
	if s.loadOrStoreBeforeLock != nil {
		s.loadOrStoreBeforeLock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false, err
	}
	if existing, ok := s.values[key]; ok {
		return existing, true, nil
	}
	s.values[key] = value
	return value, false, nil
}
