package digitaltwin

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	openTimeout   = 30 * time.Second
	retryInterval = 2 * time.Second
)

func openDatabase(appCtx context.Context) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(appCtx, os.Getenv("TIMESCALEDB_DSN"))
	if err != nil {
		return nil, fmt.Errorf("open pgx: %w", err)
	}

	ctx, cancel := context.WithTimeout(appCtx, openTimeout)
	defer cancel()

	if err := pingWithRetry(ctx, pool.Ping, retryInterval); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database after retry timeout (%s): %w", openTimeout, err)
	}

	return pool, nil
}

func pingWithRetry(ctx context.Context, ping func(context.Context) error, interval time.Duration) error {
	var lastErr error
	for {
		if err := ping(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
	}
}
