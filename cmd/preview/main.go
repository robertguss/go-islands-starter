// The synthetic login routes exist only in this loopback-only development executable.
package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	database "github.com/robertguss/go-islands-starter/db"
	"github.com/robertguss/go-islands-starter/internal/auth"
	"github.com/robertguss/go-islands-starter/internal/exports"
	"github.com/robertguss/go-islands-starter/internal/requests"
	"github.com/robertguss/go-islands-starter/internal/server"
	"github.com/robertguss/go-islands-starter/internal/store"
	"github.com/robertguss/go-islands-starter/templates"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var accounts = []struct {
	ID, Email, Name string
	Reviewer, Admin bool
}{
	{"00000000-0000-4000-8000-000000000001", "alex@example.test", "Alex Morgan", false, false},
	{"00000000-0000-4000-8000-000000000002", "blair@example.test", "Blair Chen", false, false},
	{"00000000-0000-4000-8000-000000000003", "casey@example.test", "Casey Rivera", true, false},
	{"00000000-0000-4000-8000-000000000004", "dana@example.test", "Dana Patel", false, true},
}

func main() {
	if err := run(); err != nil {
		slog.Error("preview failed", "code", err.Error())
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Fixed local credentials and database keep this fixture away from real accounts.
	// #nosec G101 -- Fixed disposable loopback-only fixture; never compiled into the production server.
	const adminURL = "postgres://postgres:local-migrations-only@127.0.0.1:55432/islands_preview?sslmode=disable"
	u, _ := url.Parse(adminURL)
	u.Path = "/islands"
	admin, err := pgxpool.New(ctx, u.String())
	if err != nil {
		return errors.New("local database unavailable")
	}
	defer admin.Close()
	var exists bool
	if err = admin.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='islands_preview')").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = admin.Exec(ctx, "CREATE DATABASE islands_preview"); err != nil {
			return err
		}
	}
	if err = database.Migrate(ctx, adminURL); err != nil {
		return err
	}
	fixture, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if _, err = fixture.Exec(ctx, "INSERT INTO app.users(id,email,display_name,requester,reviewer,access_admin) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING", a.ID, a.Email, a.Name, !a.Admin, a.Reviewer, a.Admin); err != nil {
			fixture.Close()
			return err
		}
	}
	var count int
	err = fixture.QueryRow(ctx, "SELECT count(*) FROM app.requests").Scan(&count)
	fixture.Close()
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, strings.Replace(adminURL, "postgres:local-migrations-only", "islands_app:local-runtime-only", 1))
	if err != nil {
		return err
	}
	defer st.Pool.Close()
	if count == 0 {
		titles := []string{"Refresh the team onboarding guide", "A quiet space for focused work", "New equipment for the design team", "Bring back the monthly knowledge share", "Improve the visitor welcome area", "Pilot a four-week mentoring program", "Make our meeting notes easier to find", "Update the shared resource library", "Plan the autumn team workshop", "A better way to request supplies", "Create a simple project handover checklist", "Accessibility review of shared documents"}
		service := requests.Service{Store: st}
		for i, title := range titles {
			row, e := service.Create(ctx, accounts[i%2].ID, title, "A small improvement to help our team do their best work. Please review the proposal and share your decision.", "synthetic-seed")
			if e != nil {
				return e
			}
			if i%3 != 0 {
				decision := "approved"
				if i%5 == 0 {
					decision = "rejected"
				}
				if e = service.Review(ctx, accounts[2].ID, row.ID, decision, "synthetic-seed"); e != nil {
					return e
				}
			}
		}
	}
	sessions := auth.Service{Queries: st.Auth}
	jobs, err := exports.New(st)
	if err != nil {
		return err
	}
	app := &server.Server{Store: st, Sessions: sessions, Exports: jobs, Preview: true}
	mux := http.NewServeMux()
	mux.Handle("/", app.Handler())
	mux.HandleFunc("GET /preview", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.PreviewAccounts().Render(r.Context(), w); err != nil {
			slog.Error("preview_render_failed")
		}
	})
	mux.HandleFunc("POST /preview/login", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		if r.ParseForm() != nil {
			http.Error(w, "Invalid account", http.StatusBadRequest)
			return
		}
		for _, a := range accounts {
			if a.ID == r.Form.Get("account") {
				if err := sessions.Issue(r.Context(), w, a.ID); err != nil {
					http.Error(w, "Sign-in unavailable", http.StatusInternalServerError)
					return
				}
				http.Redirect(w, r, "/requests", http.StatusSeeOther)
				return
			}
		}
		http.Error(w, "Invalid account", http.StatusBadRequest)
	})
	return server.Run(ctx, "127.0.0.1:8080", server.Headers(http.NewCrossOriginProtection().Handler(mux)), app)
}
