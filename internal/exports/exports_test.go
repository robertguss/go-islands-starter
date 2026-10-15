package exports

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/robertguss/go-islands-starter/internal/db"
	"github.com/robertguss/go-islands-starter/internal/requests"
	"github.com/robertguss/go-islands-starter/internal/testdb"
	"strings"
	"testing"
	"time"
)

func TestSafeCell(t *testing.T) {
	for _, v := range []string{"=SUM(A1)", " +123", "\tformula", "@bad", "-1", "a\nb"} {
		if !strings.HasPrefix(SafeCell(v), "'") {
			t.Errorf("unsafe cell %q", v)
		}
	}
	if SafeCell("ordinary") != "ordinary" {
		t.Fatal("changed ordinary value")
	}
}
func TestTransactionalExport(t *testing.T) {
	s, admin := testdb.New(t)
	ctx := context.Background()
	service, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	domain := requests.Service{Store: s}
	row, err := domain.Create(ctx, testdb.Alice, "=1+1", "Synthetic", "test")
	if err != nil {
		t.Fatal(err)
	}
	err = s.WithActor(ctx, testdb.Alice, func(tx pgx.Tx, q *db.Queries) error {
		ex, e := q.CreateExport(ctx, []string{row.ID})
		if e != nil {
			return e
		}
		if _, e = service.Queue.InsertTx(ctx, tx, Args{ExportID: ex.ID, ActorID: testdb.Alice}, nil); e != nil {
			return e
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	var count int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM river.river_job").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("job survived rollback")
	}
	id, err := service.Request(ctx, testdb.Alice, "test")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = Generate(ctx, s, Args{ExportID: id, ActorID: testdb.Alice}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := service.Download(ctx, testdb.Alice, id)
	if err != nil || !strings.Contains(string(data), "'=1+1") {
		t.Fatalf("export: %s %v", data, err)
	}
	if _, err = service.Download(ctx, testdb.Bob, id); err == nil {
		t.Fatal("Bob downloaded Alice export")
	}
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM app.audit_events WHERE export_id=$1 AND action='export.completed'", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate completion: %d", count)
	}
	if _, err = admin.Exec(ctx, "UPDATE app.users SET requester=false WHERE id=$1", testdb.Alice); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Download(ctx, testdb.Alice, id); err == nil {
		t.Fatal("revoked user downloaded export")
	}
	if _, err := admin.Exec(ctx, "UPDATE app.users SET requester=true WHERE id=$1", testdb.Alice); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "UPDATE app.exports SET expires_at=now()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Download(ctx, testdb.Alice, id); err == nil {
		t.Fatal("expired export downloaded")
	}
	next, err := service.Request(ctx, testdb.Alice, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Queue.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := service.Queue.Stop(stop); err != nil {
			t.Error(err)
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err = service.Download(ctx, testdb.Alice, next); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("real worker did not complete export")
}

func TestRetriesExhaustWithoutLeakingErrors(t *testing.T) {
	s, admin := testdb.New(t)
	ctx := context.Background()
	service, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (requests.Service{Store: s}).Create(ctx, testdb.Alice, "Retry fixture", "Synthetic", "test"); err != nil {
		t.Fatal(err)
	}
	id, err := service.Request(ctx, testdb.Alice, "retry-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `CREATE FUNCTION app.fail_export_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private-error-marker'; END $$;
 CREATE TRIGGER fail_export_test BEFORE UPDATE ON app.exports FOR EACH ROW WHEN (NEW.state='complete') EXECUTE FUNCTION app.fail_export_test()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := service.Queue.StopAndCancel(stop); err != nil {
			t.Error(err)
		}
	})
	if err = service.Queue.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var state, details string
		var attempts int
		if err = admin.QueryRow(ctx, "SELECT state::text, attempt, coalesce(array_to_json(errors)::text,'') FROM river.river_job WHERE args->>'export_id'=$1", id).Scan(&state, &attempts, &details); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(details, "private-error-marker") {
			t.Fatal("worker persisted a raw database error")
		}
		if state == "discarded" {
			if attempts != 3 {
				t.Fatalf("attempts=%d", attempts)
			}
			return
		}
		// Only the fixture shortens retry timing; production keeps River's backoff.
		if _, err = admin.Exec(ctx, "UPDATE river.river_job SET scheduled_at=now() WHERE args->>'export_id'=$1 AND state='retryable'", id); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("failed job did not reach its retry limit")
}

func TestInterruptedJobSurvivesWorkerRestart(t *testing.T) {
	s, admin := testdb.New(t)
	ctx := context.Background()
	service, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (requests.Service{Store: s}).Create(ctx, testdb.Alice, "Restart fixture", "Synthetic", "test"); err != nil {
		t.Fatal(err)
	}
	id, err := service.Request(ctx, testdb.Alice, "restart-test")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Rollback(context.Background()) })
	var locked string
	if err = lock.QueryRow(ctx, "SELECT id FROM app.exports WHERE id=$1 FOR UPDATE", id).Scan(&locked); err != nil {
		t.Fatal(err)
	}
	stop := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := service.Queue.StopAndCancel(cleanup); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(stop)
	if err = service.Queue.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	running := false
	for time.Now().Before(deadline) {
		var state string
		if err = admin.QueryRow(ctx, "SELECT state::text FROM river.river_job WHERE args->>'export_id'=$1", id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "running" {
			running = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !running {
		t.Fatal("worker never started blocked job")
	}
	shutdown, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	err = service.Queue.Stop(shutdown)
	cancel()
	if err == nil {
		t.Fatal("graceful stop unexpectedly completed blocked work")
	}
	stop()
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, "UPDATE river.river_job SET scheduled_at=now() WHERE args->>'export_id'=$1", id); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := restarted.Queue.StopAndCancel(cleanup); err != nil {
			t.Error(err)
		}
	})
	if err = restarted.Queue.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if _, err = restarted.Download(ctx, testdb.Alice, id); err == nil {
			var count int
			if err = admin.QueryRow(ctx, "SELECT count(*) FROM app.audit_events WHERE export_id=$1 AND action='export.completed'", id).Scan(&count); err != nil || count != 1 {
				t.Fatal("duplicate or missing completion")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("interrupted job did not survive worker restart")
}
