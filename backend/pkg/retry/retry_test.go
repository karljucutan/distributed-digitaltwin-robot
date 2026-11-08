package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoRetriesUntilSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	transient := errors.New("not ready")
	attempts := 0
	err := Do(ctx, "initialize", func(context.Context) error {
		attempts++
		if attempts < 3 {
			return transient
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("error = %v, attempts = %d; want success after 3 attempts", err, attempts)
	}
}

func TestDoCanceledBeforeAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Do(ctx, "initialize", func(context.Context) error {
		t.Fatal("operation called with canceled context")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v; want cancellation", err)
	}
}

func TestDoDeadlineDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	transient := errors.New("not ready")
	attempts := 0
	err := Do(ctx, "initialize", func(operationCtx context.Context) error {
		if operationCtx != ctx {
			t.Fatal("operation did not receive caller context")
		}
		attempts++
		return transient
	})
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, transient) || attempts != 1 {
		t.Fatalf("error = %v, attempts = %d; want deadline and last failure after 1 attempt", err, attempts)
	}
}
