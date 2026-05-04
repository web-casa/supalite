package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Pools struct {
	// RW is the main pool. Read-only enforcement is done via
	// BEGIN TRANSACTION READ ONLY at the call site, not at the pool level
	// (which can be bypassed with SET default_transaction_read_only = off).
	RW *pgxpool.Pool
}

func NewPools(ctx context.Context, databaseURL string) (*Pools, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Pools{RW: pool}, nil
}

func (p *Pools) Close() {
	p.RW.Close()
}
