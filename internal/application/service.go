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
	key := command.Tenant + "/" + command.Scope
	value := domain.NewEntity(command.ID, command.Tenant, command.Scope)
	value.Plan = command.Plan
	stored, _, err := s.store.LoadOrStore(ctx, key, value)
	if err != nil {
		return domain.Entity{}, err
	}
	return stored, nil
}
