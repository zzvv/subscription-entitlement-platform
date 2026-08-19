package repository

import (
	"context"
	"example.com/subscription-entitlement-platform/internal/domain"
	"sync"
)

type Store struct {
	mu     sync.RWMutex
	values map[string]domain.Entity
}

func NewStore() *Store { return &Store{values: map[string]domain.Entity{}} }
func (s *Store) Find(ctx context.Context, key string) (domain.Entity, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[key]
	return value, ok
}
func (s *Store) Save(ctx context.Context, key string, value domain.Entity) error {
	// 初始检查：已取消的请求不得排队等待写锁。
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 取得写锁后再次检查：请求可能在等待写锁期间被取消，
	// 此时不得继续提交新的订阅状态，避免留下半截状态。
	if err := ctx.Err(); err != nil {
		return err
	}
	s.values[key] = value
	return nil
}

// LoadOrStore atomically returns the existing value or stores value when key is absent.
func (s *Store) LoadOrStore(ctx context.Context, key string, value domain.Entity) (domain.Entity, bool, error) {
	// 初始检查：已取消的请求不得排队等待写锁。
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 取得写锁后再次检查：请求可能在等待写锁期间被取消，
	// 此时不得继续提交新的订阅状态，避免留下半截状态。
	if err := ctx.Err(); err != nil {
		return domain.Entity{}, false, err
	}
	if existing, ok := s.values[key]; ok {
		return existing, true, nil
	}
	s.values[key] = value
	return value, false, nil
}
