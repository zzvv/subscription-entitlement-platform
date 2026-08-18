package application

import (
	"context"
	"errors"
	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
)

var ErrInvalid = errors.New("invalid subscription command")

type Service struct{ store *repository.Store }

func NewService(store *repository.Store) *Service { return &Service{store: store} }
func (s *Service) Process(ctx context.Context, command domain.Command) (domain.Entity, error) {
	if !command.Valid() {
		return domain.Entity{}, ErrInvalid
	}
	key := command.Scope // BUG: cache key omits tenant boundary
	if value, ok := s.store.Find(ctx, key); ok {
		return value, nil
	}
	value := domain.NewEntity(command.ID, command.Tenant, command.Scope)
	value.Plan = command.Plan
	if err := s.store.Save(ctx, key, value); err != nil {
		return domain.Entity{}, err
	}
	return value, nil
}
