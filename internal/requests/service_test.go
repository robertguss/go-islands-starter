package requests_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/robertguss/go-islands-starter/internal/db"
	"github.com/robertguss/go-islands-starter/internal/requests"
	"github.com/robertguss/go-islands-starter/internal/testdb"
)

func TestValidation(t *testing.T) {
	for _, title := range []string{"", "   "} {
		if requests.Validate(title, "") == nil {
			t.Fatal("empty title accepted")
		}
	}
}

func TestRolesAndConcurrentReview(t *testing.T) {
	s, admin := testdb.New(t)
	ctx := context.Background()
	service := requests.Service{Store: s}
	a, err := service.Create(ctx, testdb.Alice, "Equipment request", "Synthetic record", "test")
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.Create(ctx, testdb.Bob, "Training request", "Synthetic record", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Create(ctx, testdb.Admin, "No requester role", "", "test"); !errors.Is(err, requests.ErrForbidden) {
		t.Fatalf("admin received implicit workflow permission: %v", err)
	}
	for actor, want := range map[string]int{testdb.Alice: 1, testdb.Bob: 1, testdb.Reviewer: 2, testdb.Admin: 0} {
		err = s.WithActor(ctx, actor, func(_ pgx.Tx, q *db.Queries) error {
			rows, e := q.ListRequests(ctx, "")
			if len(rows) != want {
				t.Errorf("actor %s saw %d rows, want %d", actor, len(rows), want)
			}
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = service.Review(ctx, testdb.Alice, b.ID, "approved", "test"); !errors.Is(err, requests.ErrForbidden) {
		t.Fatalf("requester reviewed: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, status := range []string{"approved", "rejected"} {
		wg.Add(1)
		go func(status string) {
			defer wg.Done()
			results <- service.Review(ctx, testdb.Reviewer, a.ID, status, "test")
		}(status)
	}
	wg.Wait()
	close(results)
	success := 0
	conflict := 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, requests.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("decisions: success=%d conflict=%d", success, conflict)
	}
	own, err := service.Create(ctx, testdb.Reviewer, "Own request", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Review(ctx, testdb.Reviewer, own.ID, "approved", "test"); !errors.Is(err, requests.ErrConflict) {
		t.Fatalf("self approval: %v", err)
	}
	var count int
	if err = admin.QueryRow(ctx, "SELECT count(*) FROM app.audit_events WHERE request_id=$1", a.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("audit count %d: %v", count, err)
	}
}
