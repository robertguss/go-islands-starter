package requests

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/robertguss/go-islands-starter/internal/db"
	"github.com/robertguss/go-islands-starter/internal/store"
)

var ErrInvalid = errors.New("invalid request")
var ErrForbidden = errors.New("not permitted")
var ErrConflict = errors.New("request is no longer pending")

type Service struct{ Store *store.Store }

func Validate(title, description string) error {
	if utf8.RuneCountInString(strings.TrimSpace(title)) < 1 || utf8.RuneCountInString(title) > 120 || utf8.RuneCountInString(description) > 2000 {
		return ErrInvalid
	}
	return nil
}

func (s Service) Create(ctx context.Context, actor, title, description, correlation string) (db.AppRequest, error) {
	var row db.AppRequest
	title = strings.TrimSpace(title)
	if err := Validate(title, description); err != nil {
		return row, err
	}
	err := s.Store.WithActor(ctx, actor, func(_ pgx.Tx, q *db.Queries) error {
		user, err := q.CurrentUser(ctx)
		if err != nil || !user.Requester {
			return ErrForbidden
		}
		row, err = q.CreateRequest(ctx, db.CreateRequestParams{Title: title, Description: description})
		if err != nil {
			return err
		}
		return q.AuditRequest(ctx, db.AuditRequestParams{Action: "request.created", RequestID: uuidValue(row.ID), CorrelationID: correlation})
	})
	return row, err
}

func (s Service) Review(ctx context.Context, actor, id, status, correlation string) error {
	if status != "approved" && status != "rejected" {
		return ErrInvalid
	}
	return s.Store.WithActor(ctx, actor, func(_ pgx.Tx, q *db.Queries) error {
		user, err := q.CurrentUser(ctx)
		if err != nil || !user.Reviewer {
			return ErrForbidden
		}
		row, err := q.ReviewRequest(ctx, db.ReviewRequestParams{ID: id, Status: status})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		return q.AuditRequest(ctx, db.AuditRequestParams{Action: "request." + status, RequestID: uuidValue(row.ID), CorrelationID: correlation})
	})
}
