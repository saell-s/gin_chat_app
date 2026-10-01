package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"gin/app/shared/configs"
)

// Mode selects the storage backend.
type Mode string

const (
	ModeMemory   Mode = "memory"
	ModePostgres Mode = "postgres"
)

// Database bundles the selected backend.
type Database struct {
	Mode Mode
	Pool *pgxpool.Pool
	Mem  *Memory
}

// Open connects to the backend described by cfg.
// In memory mode no network access is required.
func Open(ctx context.Context, cfg configs.DatabaseConfig) (*Database, error) {
	switch Mode(cfg.Mode) {
	case ModePostgres:
		if cfg.URL == "" {
			return nil, fmt.Errorf("DB_MODE=postgres requires DATABASE_URL")
		}
		poolCfg, err := pgxpool.ParseConfig(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
		}
		if cfg.MaxConns > 0 {
			poolCfg.MaxConns = cfg.MaxConns
		}
		pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
		if err != nil {
			return nil, fmt.Errorf("connect postgres: %w", err)
		}
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		}
		return &Database{Mode: ModePostgres, Pool: pool}, nil
	case ModeMemory, "":
		return &Database{Mode: ModeMemory, Mem: NewMemory()}, nil
	default:
		return nil, fmt.Errorf("unknown DB_MODE %q (use memory or postgres)", cfg.Mode)
	}
}

// Close releases backend resources.
func (d *Database) Close() {
	if d == nil {
		return
	}
	if d.Pool != nil {
		d.Pool.Close()
	}
	if d.Mem != nil {
		d.Mem.Close()
	}
}

// IsPostgres reports whether SQL queries should be used.
func (d *Database) IsPostgres() bool { return d != nil && d.Mode == ModePostgres }

// String describes the active backend for logs.
func (d *Database) String() string {
	if d.IsPostgres() {
		return "postgres"
	}
	return "memory"
}

// Migrate applies the schema and seeds data when the database is empty.
func (d *Database) Migrate(ctx context.Context, cfg configs.DatabaseConfig, hash func(string) (string, error)) error {
	if !d.IsPostgres() {
		if cfg.Seed {
			SeedMemory(d.Mem, hash)
		}
		return nil
	}
	if !cfg.AutoMigrate {
		return nil
	}
	if err := migrateSchema(ctx, d.Pool); err != nil {
		return err
	}
	if cfg.Seed {
		return seedPostgres(ctx, d.Pool, hash)
	}
	return nil
}

// WaitReady logs a warning when the backend is not reachable (never fatal in memory mode).
func (d *Database) WaitReady(logf func(string, ...any)) {
	if d.IsPostgres() {
		logf("database: connected to postgres")
		return
	}
	logf("database: using in-memory store (set DATABASE_URL to use postgres)")
	_ = log.Default()
}
