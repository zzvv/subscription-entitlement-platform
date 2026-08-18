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
func (s *Store) Find(_ context.Context, key string) (domain.Entity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[key]
	return value, ok
}
func (s *Store) Save(_ context.Context, key string, value domain.Entity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}
