package store_test

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robertguss/go-islands-starter/internal/db"
	"github.com/robertguss/go-islands-starter/internal/requests"
	"github.com/robertguss/go-islands-starter/internal/store"
	"github.com/robertguss/go-islands-starter/internal/testdb"
	"testing"
)

func TestRLSAndConnectionReuse(t *testing.T) {
	original, admin := testdb.New(t)
	ctx := context.Background()
	cfg := original.Pool.Config()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s := &store.Store{Pool: pool, Auth: db.New(pool)}
	service := requests.Service{Store: s}
	row, err := service.Create(ctx, testdb.Alice, "Private request", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"UPDATE app.requests SET owner_id='" + testdb.Bob + "' WHERE id='" + row.ID + "'",
		"DELETE FROM app.requests WHERE id='" + row.ID + "'",
		"UPDATE app.audit_events SET action='export.failed'",
		"DELETE FROM app.audit_events",
		"UPDATE app.users SET reviewer=true WHERE id='" + testdb.Alice + "'",
		"ALTER TABLE app.requests DISABLE ROW LEVEL SECURITY",
		"INSERT INTO app.requests(owner_id,title,description) VALUES ('" + testdb.Bob + "','spoof','')",
	} {
		err = s.WithActor(ctx, testdb.Alice, func(tx pgx.Tx, _ *db.Queries) error { _, e := tx.Exec(ctx, statement); return e })
		if err == nil {
			t.Fatalf("unexpectedly allowed %s", statement)
		}
	}
	if _, err = store.Open(ctx, admin.Config().ConnString()); err == nil {
		t.Fatal("accepted privileged runtime credentials")
	}
	for _, mode := range []string{"commit", "rollback", "cancel"} {
		callCtx, cancel := context.WithCancel(ctx)
		err = s.WithActor(callCtx, testdb.Alice, func(_ pgx.Tx, q *db.Queries) error {
			if _, e := q.ListRequests(callCtx, ""); e != nil {
				return e
			}
			if mode == "rollback" {
				return errors.New("forced rollback")
			}
			if mode == "cancel" {
				cancel()
				return callCtx.Err()
			}
			return nil
		})
		cancel()
		if mode == "commit" && err != nil {
			t.Fatal(err)
		}
		if mode != "commit" && err == nil {
			t.Fatal("failure unexpectedly committed")
		}
		rows, e := db.New(pool).ListRequests(ctx, "")
		if e != nil || len(rows) != 0 {
			t.Fatalf("%s leaked context: %d %v", mode, len(rows), e)
		}
		err = s.WithActor(ctx, testdb.Bob, func(_ pgx.Tx, q *db.Queries) error {
			rows, e := q.ListRequests(ctx, "")
			if len(rows) != 0 {
				t.Error("Bob saw Alice's data")
			}
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
