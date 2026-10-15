package exports

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/robertguss/go-islands-starter/internal/db"
	"github.com/robertguss/go-islands-starter/internal/requests"
	"github.com/robertguss/go-islands-starter/internal/store"
)

type Args struct {
	ExportID string `json:"export_id"`
	ActorID  string `json:"actor_id"`
}

func (Args) Kind() string { return "export_csv" }

type Service struct {
	Store *store.Store
	Queue *river.Client[pgx.Tx]
}
type worker struct {
	river.WorkerDefaults[Args]
	store *store.Store
}

func New(s *store.Store) (*Service, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &worker{store: s})
	client, err := river.NewClient(riverpgxv5.New(s.Pool), &river.Config{Schema: "river", Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}}, MaxAttempts: 3, JobTimeout: 30 * time.Second, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		return nil, err
	}
	return &Service{Store: s, Queue: client}, nil
}

func (s *Service) Request(ctx context.Context, actor, correlation string) (string, error) {
	var id string
	err := s.Store.WithActor(ctx, actor, func(tx pgx.Tx, q *db.Queries) error {
		user, err := q.CurrentUser(ctx)
		if err != nil || (!user.Requester && !user.Reviewer) {
			return requests.ErrForbidden
		}
		rows, err := q.ListRequests(ctx, "")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return requests.ErrInvalid
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		ex, err := q.CreateExport(ctx, ids)
		if err != nil {
			return err
		}
		id = ex.ID
		if err = q.AuditExport(ctx, db.AuditExportParams{Action: "export.requested", ExportID: uuidValue(id), CorrelationID: correlation}); err != nil {
			return err
		}
		job, err := s.Queue.InsertTx(ctx, tx, Args{ExportID: id, ActorID: actor}, nil)
		if err != nil {
			return err
		}
		return q.SetExportJob(ctx, db.SetExportJobParams{ID: id, JobID: pgtype.Int8{Int64: job.Job.ID, Valid: true}})
	})
	return id, err
}

func (w *worker) Work(ctx context.Context, job *river.Job[Args]) error {
	return Generate(ctx, w.store, job.Args)
}

func Generate(ctx context.Context, s *store.Store, args Args) error {
	denied := false
	err := s.WithActor(ctx, args.ActorID, func(_ pgx.Tx, q *db.Queries) error {
		ex, err := q.GetExport(ctx, args.ExportID)
		if errors.Is(err, pgx.ErrNoRows) {
			denied = true
			return nil
		}
		if err != nil {
			return err
		}
		if ex.State != "pending" {
			return nil
		}
		rows, err := q.RequestsByID(ctx, ex.RequestIds)
		if err != nil {
			return err
		}
		if len(rows) != len(ex.RequestIds) || time.Now().After(ex.ExpiresAt) {
			denied = true
			if err = q.FailExport(ctx, ex.ID); err != nil {
				return err
			}
			return q.AuditExport(ctx, db.AuditExportParams{Action: "export.failed", ExportID: uuidValue(ex.ID), CorrelationID: ex.ID})
		}
		data, err := CSV(rows)
		if err != nil {
			return err
		}
		if err = q.CompleteExport(ctx, db.CompleteExportParams{ID: ex.ID, Content: data}); err != nil {
			return err
		}
		return q.AuditExport(ctx, db.AuditExportParams{Action: "export.completed", ExportID: uuidValue(ex.ID), CorrelationID: ex.ID})
	})
	if denied {
		return river.JobCancel(errors.New("export no longer permitted or expired"))
	}
	if err != nil {
		return errors.New("export processing failed")
	}
	return nil
}

func (s *Service) Download(ctx context.Context, actor, id string) ([]byte, error) {
	var result []byte
	err := s.Store.WithActor(ctx, actor, func(_ pgx.Tx, q *db.Queries) error {
		if _, err := q.CurrentUser(ctx); err != nil {
			return requests.ErrForbidden
		}
		ex, err := q.GetExport(ctx, id)
		if err != nil {
			return requests.ErrForbidden
		}
		if ex.State != "complete" || time.Now().After(ex.ExpiresAt) {
			return requests.ErrForbidden
		}
		rows, err := q.RequestsByID(ctx, ex.RequestIds)
		if err != nil {
			return err
		}
		if len(rows) != len(ex.RequestIds) {
			return requests.ErrForbidden
		}
		result = ex.Content
		return nil
	})
	return result, err
}

func SafeCell(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if strings.ContainsAny(value, "\t\r\n") || (trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + value
	}
	return value
}
func CSV(rows []db.AppRequest) ([]byte, error) {
	if len(rows) > 200 {
		return nil, requests.ErrInvalid
	}
	var out bytes.Buffer
	w := csv.NewWriter(&out)
	if err := w.Write([]string{"ID", "Title", "Description", "Status", "Created (UTC)"}); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if err := w.Write([]string{r.ID, SafeCell(r.Title), SafeCell(r.Description), r.Status, r.CreatedAt.UTC().Format(time.RFC3339)}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	if out.Len() > 1<<20 {
		return nil, requests.ErrInvalid
	}
	return out.Bytes(), nil
}
func uuidValue(id string) pgtype.UUID { var v pgtype.UUID; _ = v.Scan(id); return v }
