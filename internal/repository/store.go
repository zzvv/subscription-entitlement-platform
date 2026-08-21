package repository

import (
	"context"
	"example.com/subscription-entitlement-platform/internal/domain"
	"sync"
)

type Store struct {
	mu                    sync.RWMutex
	values                map[string]domain.Entity
	versions              map[string]uint64
	receipts              map[string]domain.Receipt
	receiptCommitErr      error
	loadOrStoreBeforeLock func()
	commitBeforeLock      func()
}

func NewStore() *Store {
	return &Store{
		values:   map[string]domain.Entity{},
		versions: map[string]uint64{},
		receipts: map[string]domain.Receipt{},
	}
}
func (s *Store) Find(ctx context.Context, key string) (domain.Entity, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[key]
	return value, ok
}

// FindWithVersion returns the value together with the store version it was read
// at. The version is captured under the store lock, so a caller can use it to
// detect that a concurrent Save committed a newer state after this read.
func (s *Store) FindWithVersion(ctx context.Context, key string) (domain.Entity, uint64, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, 0, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[key]
	if !ok {
		return domain.Entity{}, 0, false
	}
	return value, s.versions[key], true
}

func (s *Store) Save(ctx context.Context, key string, value domain.Entity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	s.versions[key] = s.versions[key] + 1
	return nil
}

// SetLoadOrStoreBeforeLockForTest pauses LoadOrStore after its initial context check.
func (s *Store) SetLoadOrStoreBeforeLockForTest(hook func()) {
	s.loadOrStoreBeforeLock = hook
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
	s.versions[key] = 1
	return value, false, nil
}
