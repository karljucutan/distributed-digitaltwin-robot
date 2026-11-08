package retry

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"
)

// Do retries all errors with jittered exponential backoff, from 250ms up to 5s.
// The caller's context controls the overall deadline and cancellation.
func Do(ctx context.Context, name string, operation func(context.Context) error) error {
	delay := 250 * time.Millisecond
	var lastErr error
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return fmt.Errorf("%s: %w (last failure: %w)", name, err, lastErr)
			}
			return fmt.Errorf("%s: %w", name, err)
		}
		err := operation(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		wait := delay/2 + rand.N(delay/2)
		slog.Warn("retrying operation", "operation", name, "attempt", attempt, "delay", wait, "error", err)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		delay = min(delay*2, 5*time.Second)
	}
}
