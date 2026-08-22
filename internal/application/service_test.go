package application

import (
	"context"
	"errors"
	"testing"

	"example.com/subscription-entitlement-platform/internal/domain"
	"example.com/subscription-entitlement-platform/internal/repository"
)

func TestProcessPropagatesCanceledContext(t *testing.T) {
	service := NewService(repository.NewStore())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.Process(ctx, domain.NewCommand("sub-a", "tenant-a", "standard", "activate"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestProcessKeepsTenantStateIsolated(t *testing.T) {
	service := NewService(repository.NewStore())
	first, err := service.Process(context.Background(), domain.NewCommand("sub-a", "tenant-a", "standard", "activate"))
	if err != nil {
		t.Fatalf("process tenant-a command: %v", err)
	}
	second, err := service.Process(context.Background(), domain.NewCommand("sub-b", "tenant-b", "standard", "activate"))
	if err != nil {
		t.Fatalf("process tenant-b command: %v", err)
	}
	if first.ID == second.ID || second.Tenant != "tenant-b" {
		t.Fatalf("tenant-b received tenant-a state: first=%+v second=%+v", first, second)
	}
}

func TestProcessIsStableForConcurrentCommands(t *testing.T) {
	service := NewService(repository.NewStore())
	const workers = 32
	results := make(chan domain.Entity, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func(i int) {
			<-start
			value, err := service.Process(context.Background(), domain.NewCommand(string(rune('a'+i)), "tenant-a", "standard", "activate"))
			if err != nil {
				t.Errorf("process failed: %v", err)
				return
			}
			results <- value
		}(i)
	}
	close(start)
	first := <-results
	for i := 1; i < workers; i++ {
		if got := <-results; got != first {
			t.Fatalf("concurrent process returned different values: first=%+v got=%+v", first, got)
		}
	}
}
