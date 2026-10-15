package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robertguss/go-islands-starter/internal/auth"
	"github.com/robertguss/go-islands-starter/internal/db"
	"github.com/robertguss/go-islands-starter/internal/exports"
	"github.com/robertguss/go-islands-starter/internal/testdb"
)

func TestHTTPBoundaries(t *testing.T) {
	st, fixture := testdb.New(t)
	ctx := context.Background()
	jobs, err := exports.New(st)
	if err != nil {
		t.Fatal(err)
	}
	app := &Server{Store: st, Exports: jobs, Sessions: auth.Service{Queries: st.Auth}}
	cookie := func(actor string) *http.Cookie {
		w := httptest.NewRecorder()
		if err := app.Sessions.Issue(ctx, w, actor); err != nil {
			t.Fatal(err)
		}
		return w.Result().Cookies()[0]
	}
	alice, admin := cookie(testdb.Alice), cookie(testdb.Admin)
	call := func(method, path, form string, c *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://app.test"+path, strings.NewReader(form))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if c != nil {
			r.AddCookie(c)
		}
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		return w
	}
	if got := call("POST", "/requests", "title=Hello", alice, map[string]string{"Origin": "https://evil.test", "Sec-Fetch-Site": "cross-site"}); got.Code != 403 {
		t.Fatal("cross-site form accepted")
	}
	if got := call("POST", "/requests", "title=Hello", alice, map[string]string{"Origin": "http://app.test", "Sec-Fetch-Site": "same-origin"}); got.Code != 303 {
		t.Fatalf("same-origin=%d", got.Code)
	}
	for _, hx := range []string{"", "true"} {
		if got := call("POST", "/requests", "title=No", admin, map[string]string{"HX-Request": hx}); got.Code != 403 {
			t.Fatal("admin got implicit workflow access")
		}
		if got := call("GET", "/people", "", alice, map[string]string{"HX-Request": hx}); got.Code != 403 {
			t.Fatal("requester accessed people")
		}
	}
	if got := call("POST", "/requests", "title=", alice, nil); got.Code != 400 {
		t.Fatal("invalid form accepted")
	}
	if got := call("POST", "/requests", "title="+strings.Repeat("x", 17000), alice, nil); got.Code != 400 {
		t.Fatal("oversized form accepted")
	}
	if got := call("POST", "/admin/synthetic-operation", "", admin, nil); got.Code != 403 {
		t.Fatal("destructive gate default on")
	}
	app.DestructiveEnabled = true
	if got := call("POST", "/admin/synthetic-operation", "", alice, nil); got.Code != 403 {
		t.Fatal("gate bypassed permission")
	}
	if got := call("POST", "/admin/synthetic-operation", "", admin, nil); got.Code != 200 {
		t.Fatal("authorized synthetic gate failed")
	}
	if got := call("GET", "/dashboard", "", alice, nil); got.Code != 200 || !strings.Contains(got.Body.String(), "data-island=\"dashboard\"") || got.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("dashboard contract")
	}
	// A profile form cannot smuggle roles through request creation.
	if got := call("POST", "/requests", "title=Untrusted&reviewer=true&owner_id="+testdb.Bob, alice, nil); got.Code != 303 {
		t.Fatal("request failed")
	}
	var elevated bool
	if err = fixture.QueryRow(ctx, "SELECT reviewer FROM app.users WHERE id=$1", testdb.Alice).Scan(&elevated); err != nil || elevated {
		t.Fatal("role escalation")
	}
	// Terminal worker failure must be visible without exposing internal errors.
	exportID, err := jobs.Request(ctx, testdb.Alice, "failed-job-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.Exec(ctx, "UPDATE river.river_job SET state='discarded', finalized_at=now(), attempt=3 WHERE args->>'export_id'=$1", exportID); err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "/exports", "", alice, nil); got.Code != 200 || !strings.Contains(got.Body.String(), "Could not complete") {
		t.Fatal("terminal failure not visible")
	}
	// The database independently denies the privileged function to non-admins.
	if err = st.WithActor(ctx, testdb.Alice, func(_ pgx.Tx, q *db.Queries) error {
		return q.ManageUser(ctx, db.ManageUserParams{Email: "new@example.test", DisplayName: "New", AccessAdmin: true, Correlation: "test"})
	}); err == nil {
		t.Fatal("direct administration bypass")
	}
	if got := call("POST", "/people", "email=alex%40example.test&name=Alex&requester=on&disabled=on", admin, nil); got.Code != 303 {
		t.Fatalf("admin change status=%d", got.Code)
	}
	if got := call("GET", "/requests", "", alice, nil); got.Code != 303 || got.Header().Get("Location") != "/" {
		t.Fatal("disabled session remains active")
	}
	var events int
	if err = fixture.QueryRow(ctx, "SELECT count(*) FROM app.audit_events WHERE action='user.updated'").Scan(&events); err != nil || events != 1 {
		t.Fatal("access change missing audit")
	}
}
func TestLimiter(t *testing.T) {
	l := newLimiter(2)
	now := time.Now()
	first, second := l.allow("a", now), l.allow("a", now)
	if !first || !second || l.allow("a", now) || !l.allow("b", now) || !l.allow("a", now.Add(time.Minute)) {
		t.Fatal("rate limit")
	}
}
