package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	sharedpostgres "github.com/jur4ikoff/utils/postgres"
)

func NewPool(
	ctx context.Context, databaseURL string, maxConns int32, minConns int32,
) (*pgxpool.Pool, error) {
	pool, err := sharedpostgres.NewPool(ctx, databaseURL, maxConns, minConns)
	if err != nil {
		return nil, fmt.Errorf("failed to create PostgreSQL pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping PostreSql: %w", err)
	}

	return pool, nil
}
