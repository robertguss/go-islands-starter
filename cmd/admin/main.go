// Bootstrap is an operator-only command; never pass its credentials to the server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/mail"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	email := flag.String("email", "", "first administrator email")
	name := flag.String("name", "", "display name")
	flag.Parse()
	*email = strings.ToLower(strings.TrimSpace(*email))
	a, err := mail.ParseAddress(*email)
	if err != nil || a.Address != *email || *name == "" || len(*name) > 120 {
		return errors.New("provide a valid -email and -name")
	}
	raw := os.Getenv("MIGRATION_DATABASE_URL")
	if raw == "" {
		return errors.New("MIGRATION_DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, raw)
	if err != nil {
		return errors.New("operator connection failed")
	}
	defer func() { _ = conn.Close(context.Background()) }()
	// Infrastructure exception: bootstrap runs before any actor has access.
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7240183)"); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `INSERT INTO app.users(email,display_name,access_admin) SELECT $1,$2,true WHERE NOT EXISTS (SELECT 1 FROM app.users WHERE access_admin AND NOT disabled)`, *email, *name)
	if err != nil {
		return errors.New("bootstrap failed")
	}
	if result.RowsAffected() != 1 {
		return errors.New("an administrator already exists; use People to manage access")
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("bootstrap commit failed")
	}
	fmt.Println("First access administrator admitted.")
	return nil
}
