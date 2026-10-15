package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robertguss/go-islands-starter/internal/auth"
	"github.com/robertguss/go-islands-starter/internal/dashboard"
	"github.com/robertguss/go-islands-starter/internal/db"
	"github.com/robertguss/go-islands-starter/internal/exports"
	"github.com/robertguss/go-islands-starter/internal/requests"
	"github.com/robertguss/go-islands-starter/internal/store"
	"github.com/robertguss/go-islands-starter/templates"
	"github.com/robertguss/go-islands-starter/web"
)

type Server struct {
	Store                       *store.Store
	Sessions                    auth.Service
	OIDC                        *auth.OIDC
	Exports                     *exports.Service
	Preview, DestructiveEnabled bool
}

type actorHandler func(http.ResponseWriter, *http.Request, db.AppUser) error

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(web.Assets())))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := s.Store.Pool.Ping(ctx); err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { render(w, r, 200, templates.SignIn(s.Preview)) })
	if s.OIDC != nil {
		loginLimit := newLimiter(10)
		mux.Handle("GET /auth/login", loginLimit.Wrap(http.HandlerFunc(s.OIDC.Start)))
		mux.HandleFunc("GET /auth/callback", s.OIDC.Callback)
	}
	mux.HandleFunc("POST /auth/logout", s.Sessions.Logout)
	mux.HandleFunc("GET /requests", s.protected(s.listRequests))
	mux.HandleFunc("POST /requests", s.protected(s.createRequest))
	mux.HandleFunc("POST /requests/{id}/review", s.protected(s.reviewRequest))
	mux.HandleFunc("GET /dashboard", s.protected(s.overview))
	mux.HandleFunc("GET /activity", s.protected(s.activity))
	mux.HandleFunc("GET /exports", s.protected(s.listExports))
	expensive := newLimiter(5)
	mux.Handle("POST /exports", expensive.Wrap(http.HandlerFunc(s.protected(s.createExport))))
	mux.HandleFunc("GET /exports/{id}", s.protected(s.download))
	mux.HandleFunc("GET /people", s.protected(s.people))
	mux.HandleFunc("POST /people", s.protected(s.managePerson))
	mux.HandleFunc("POST /admin/synthetic-operation", s.protected(func(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
		if !u.AccessAdmin || !s.DestructiveEnabled {
			return requests.ErrForbidden
		}
		_, err := w.Write([]byte("Synthetic operation completed. No external system was changed."))
		return err
	}))
	return Headers(http.NewCrossOriginProtection().Handler(mux))
}

func (s *Server) protected(next actorHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.Sessions.User(r)
		if err != nil {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/")
				w.WriteHeader(401)
			} else {
				http.Redirect(w, r, "/", http.StatusSeeOther)
			}
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err = next(w, r, u); err == nil {
			return
		}
		status, title, message := 500, "Something went wrong", "Please retry in a moment."
		switch {
		case errors.Is(err, requests.ErrInvalid):
			status, title, message = 400, "Check your details", "Use a title of 1–120 characters and details of no more than 2,000 characters. Check that all required fields are filled in."
		case errors.Is(err, requests.ErrForbidden):
			status, title, message = 403, "Access unavailable", "You don't have permission for this action. Your access may have changed."
		case errors.Is(err, requests.ErrConflict):
			status, title, message = 409, "Already decided", "This request has already been reviewed, or you cannot review your own request."
		}
		render(w, r, status, templates.ErrorPage(s.page(u, "Requests", r), title, message))
	}
}
func (s *Server) page(u db.AppUser, active string, r *http.Request) templates.Page {
	message := map[string]string{"created": "Request submitted. Your reviewer can take it from here.", "reviewed": "Decision saved.", "exported": "Export queued. Refresh shortly to download your file.", "updated": "Access updated. Active sessions have been ended."}[r.URL.Query().Get("notice")]
	return templates.Page{User: u, Active: active, Preview: s.Preview, Message: message}
}
func render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	var b bytes.Buffer
	if err := c.Render(r.Context(), &b); err != nil {
		http.Error(w, "Page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b.Bytes())
}
func (s *Server) read(r *http.Request, u db.AppUser, fn func(*db.Queries) error) error {
	return s.Store.WithActor(r.Context(), u.ID, func(_ pgx.Tx, q *db.Queries) error { return fn(q) })
}
func (s *Server) listRequests(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 480 {
		return requests.ErrInvalid
	}
	var rows []db.AppRequest
	err := s.read(r, u, func(q *db.Queries) error { var e error; rows, e = q.ListRequests(r.Context(), search); return e })
	if err != nil {
		return err
	}
	if r.Header.Get("HX-Request") == "true" {
		render(w, r, 200, templates.SearchResults(u, rows))
	} else {
		render(w, r, 200, templates.Requests(s.page(u, "Requests", r), rows, search))
	}
	return nil
}
func (s *Server) createRequest(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	if err := r.ParseForm(); err != nil {
		return requests.ErrInvalid
	}
	title, description := r.Form.Get("title"), r.Form.Get("description")
	if err := requests.Validate(title, description); err != nil {
		render(w, r, 400, templates.RequestValidation(s.page(u, "Requests", r), title, description))
		return nil
	}
	_, err := (requests.Service{Store: s.Store}).Create(r.Context(), u.ID, r.Form.Get("title"), r.Form.Get("description"), w.Header().Get("X-Request-ID"))
	if err != nil {
		return err
	}
	http.Redirect(w, r, "/requests?notice=created", http.StatusSeeOther)
	return nil
}
func validID(id string) bool { var v pgtype.UUID; return v.Scan(id) == nil && v.Valid }
func (s *Server) reviewRequest(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	if !validID(r.PathValue("id")) || r.ParseForm() != nil {
		return requests.ErrInvalid
	}
	err := (requests.Service{Store: s.Store}).Review(r.Context(), u.ID, r.PathValue("id"), r.Form.Get("decision"), w.Header().Get("X-Request-ID"))
	if err != nil {
		return err
	}
	http.Redirect(w, r, "/requests?notice=reviewed", http.StatusSeeOther)
	return nil
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	props := dashboard.Props{Requests: []dashboard.Request{}}
	err := s.read(r, u, func(q *db.Queries) error {
		rows, e := q.ListRequests(r.Context(), "")
		if e != nil {
			return e
		}
		for _, row := range rows {
			props.Requests = append(props.Requests, dashboard.Request{ID: row.ID, Title: row.Title, Status: row.Status, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339)})
		}
		return nil
	})
	if err != nil {
		return err
	}
	if r.Header.Get("HX-Request") == "true" {
		render(w, r, 200, templates.DashboardPanel(props))
	} else {
		render(w, r, 200, templates.Overview(s.page(u, "Overview", r), props))
	}
	return nil
}
func (s *Server) activity(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	var rows []db.AppAuditEvent
	err := s.read(r, u, func(q *db.Queries) error { var e error; rows, e = q.ListAudit(r.Context()); return e })
	if err != nil {
		return err
	}
	render(w, r, 200, templates.Activity(s.page(u, "Activity", r), rows))
	return nil
}
func (s *Server) createExport(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	_, err := s.Exports.Request(r.Context(), u.ID, w.Header().Get("X-Request-ID"))
	if err != nil {
		return err
	}
	http.Redirect(w, r, "/exports?notice=exported", http.StatusSeeOther)
	return nil
}
func (s *Server) listExports(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	var rows []db.ListExportsRow
	err := s.read(r, u, func(q *db.Queries) error { var e error; rows, e = q.ListExports(r.Context()); return e })
	if err != nil {
		return err
	}
	result := make([]templates.Export, 0, len(rows))
	for _, row := range rows {
		state := row.State
		if time.Now().After(row.ExpiresAt) {
			state = "expired"
		} else if state == "pending" && row.JobID.Valid {
			job, e := s.Exports.Queue.JobGet(r.Context(), row.JobID.Int64)
			if e != nil {
				return e
			}
			if job.State == "cancelled" || job.State == "discarded" {
				state = "failed"
			}
		}
		result = append(result, templates.Export{ID: row.ID, State: state, CreatedAt: row.CreatedAt})
	}
	render(w, r, 200, templates.Exports(s.page(u, "Exports", r), result))
	return nil
}
func (s *Server) download(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	if !validID(r.PathValue("id")) {
		return requests.ErrInvalid
	}
	data, err := s.Exports.Download(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="requests.csv"`)
	// #nosec G705 -- CSV attachment with explicit text/csv and nosniff, never an HTML response.
	_, err = w.Write(data)
	return err
}
func (s *Server) people(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	if !u.AccessAdmin {
		return requests.ErrForbidden
	}
	var users []db.ListUsersRow
	err := s.read(r, u, func(q *db.Queries) error { var e error; users, e = q.ListUsers(r.Context()); return e })
	if err != nil {
		return err
	}
	render(w, r, 200, templates.People(s.page(u, "People", r), users))
	return nil
}
func (s *Server) managePerson(w http.ResponseWriter, r *http.Request, u db.AppUser) error {
	if !u.AccessAdmin {
		return requests.ErrForbidden
	}
	if r.ParseForm() != nil {
		return requests.ErrInvalid
	}
	email, name := strings.ToLower(strings.TrimSpace(r.Form.Get("email"))), strings.TrimSpace(r.Form.Get("name"))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || email == u.Email || name == "" || len(name) > 120 || len(email) > 254 {
		return requests.ErrInvalid
	}
	err = s.read(r, u, func(q *db.Queries) error {
		return q.ManageUser(r.Context(), db.ManageUserParams{Email: email, DisplayName: name, Requester: r.Form.Get("requester") == "on", Reviewer: r.Form.Get("reviewer") == "on", AccessAdmin: r.Form.Get("admin") == "on", Disabled: r.Form.Get("disabled") == "on", Correlation: w.Header().Get("X-Request-ID")})
	})
	if err != nil {
		return err
	}
	http.Redirect(w, r, "/people?notice=updated", http.StatusSeeOther)
	return nil
}
