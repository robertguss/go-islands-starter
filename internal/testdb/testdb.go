// Package testdb provisions disposable synthetic databases for integration tests.
package testdb

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	database "github.com/robertguss/go-islands-starter/db"
	"github.com/robertguss/go-islands-starter/internal/store"
)

const Alice = "00000000-0000-4000-8000-000000000001"
const Bob = "00000000-0000-4000-8000-000000000002"
const Reviewer = "00000000-0000-4000-8000-000000000003"
const Admin = "00000000-0000-4000-8000-000000000004"

func New(t *testing.T) (*store.Store, *pgxpool.Pool) {
	t.Helper()
	raw := os.Getenv("ISLANDS_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("set ISLANDS_TEST_DATABASE_URL to the dedicated local Postgres admin URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
		t.Fatal("integration tests require a loopback database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := "islands_test_" + strings.ToLower(rand.Text()[:12])
	var runtime, fixture *pgxpool.Pool
	t.Cleanup(func() {
		if runtime != nil {
			runtime.Close()
		}
		if fixture != nil {
			fixture.Close()
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("test database cleanup: %v", err)
		}
	})
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	if err = database.Migrate(ctx, u.String()); err != nil {
		t.Fatal(err)
	}
	fixture, err = pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.Exec(ctx, `INSERT INTO app.users(id,email,display_name,requester,reviewer,access_admin) VALUES
 ($1,'alex@example.test','Alex Morgan',true,false,false),($2,'blair@example.test','Blair Chen',true,false,false),
 ($3,'casey@example.test','Casey Rivera',true,true,false),($4,'dana@example.test','Dana Patel',false,false,true)`, Alice, Bob, Reviewer, Admin); err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword("islands_app", "local-runtime-only")
	s, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	runtime = s.Pool
	return s, fixture
}
