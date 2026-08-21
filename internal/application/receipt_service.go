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
	// The subscription ID is only unique within a tenant, so the receipt ID must
	// be scoped by tenant too. Otherwise two tenants reusing the same subscription
	// ID would share one receipt key, and the later confirmation would overwrite
	// the earlier tenant's notification receipt (and a lookup could return another
	// tenant's record). Within a single tenant the resulting key stays stable, so
	// repeated confirmations keep behaving idempotently.
	receiptID := command.Tenant + "/" + command.ID + "/" + command.Action
	receipt := domain.NewReceipt(receiptID, command.Tenant, command.ID, command.Action)
	if err := s.store.CommitWithReceipt(ctx, key, value, receipt); err != nil {
		return domain.Entity{}, err
	}
	return value, nil
}
