package application

import (
	"context"
	"errors"
	"testing"
	"time"

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

// cancelAfterFirstCheckContext 在首次 ctx.Err() 检查时返回 nil，之后返回 context.Canceled，
// 用来模拟订阅命令在写入流程中途（取得写锁后）被取消的场景。
type cancelAfterFirstCheckContext struct{ checks int }

func (c *cancelAfterFirstCheckContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterFirstCheckContext) Done() <-chan struct{}       { return nil }
func (c *cancelAfterFirstCheckContext) Err() error {
	c.checks++
	if c.checks > 1 {
		return context.Canceled
	}
	return nil
}
func (c *cancelAfterFirstCheckContext) Value(any) any { return nil }

// TestProcessDoesNotCommitAfterMidFlowCancellation 覆盖订阅命令处理流程：
// 命令通过校验并进入仓储写入后，context 在取得写锁后取消时，不得留下可读取的新状态。
func TestProcessDoesNotCommitAfterMidFlowCancellation(t *testing.T) {
	store := repository.NewStore()
	service := NewService(store)
	ctx := &cancelAfterFirstCheckContext{}
	_, err := service.Process(ctx, domain.NewCommand("sub-a", "tenant-a", "standard", "activate"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected mid-flow cancellation, got %v", err)
	}
	// 幂等读取不得观察到已取消操作留下的状态。
	if _, ok := store.Find(context.Background(), "tenant-a/standard"); ok {
		t.Fatal("canceled command must not leave readable subscription state")
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
