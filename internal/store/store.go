package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robertguss/go-islands-starter/internal/db"
)

type Store struct {
	Pool *pgxpool.Pool
	Auth *db.Queries
}

func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	cfg.MaxConns = 8
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "10000"
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("database initialization failed")
	}
	var unsafe bool
	err = p.QueryRow(ctx, `SELECT r.rolsuper OR r.rolbypassrls OR r.rolcreaterole OR r.rolcreatedb OR r.rolreplication OR
 EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname='app' AND pg_has_role(current_user,c.relowner,'MEMBER'))
 FROM pg_roles r WHERE r.rolname=current_user`).Scan(&unsafe)
	if err != nil || unsafe {
		p.Close()
		return nil, errors.New("database unavailable or runtime role has excessive privileges")
	}
	return &Store{Pool: p, Auth: db.New(p)}, nil
}

func (s *Store) WithActor(ctx context.Context, actor string, fn func(pgx.Tx, *db.Queries) error) error {
	if actor == "" {
		return errors.New("actor required")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	if err = q.SetActor(ctx, actor); err != nil {
		return err
	}
	if err = fn(tx, q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
