package application

import (
	"context"
	"errors"

	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
)

var ErrInvalidConfirmation = errors.New("invalid subscription confirmation")

// ReceiptService confirms a subscription state change and schedules its notification receipt.
type ReceiptService struct{ store *repository.Store }

func NewReceiptService(store *repository.Store) *ReceiptService { return &ReceiptService{store: store} }

func (s *ReceiptService) Confirm(ctx context.Context, command domain.Command) (domain.Entity, error) {
	if !command.Valid() || command.Action != "activate" {
		return domain.Entity{}, ErrInvalidConfirmation
	}
	key := command.Tenant + "/" + command.Scope
	value := domain.NewEntity(command.ID, command.Tenant, command.Scope)
	value.Plan = command.Plan
	receipt := domain.NewReceipt(command.ID+"/activate", command.Tenant, command.ID, command.Action)
	if err := s.store.CommitWithReceipt(ctx, key, value, receipt); err != nil {
		return domain.Entity{}, err
	}
	return value, nil
}
