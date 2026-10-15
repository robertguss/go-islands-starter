package main

import (
	"context"
	"github.com/robertguss/go-islands-starter/internal/auth"
	"github.com/robertguss/go-islands-starter/internal/config"
	"github.com/robertguss/go-islands-starter/internal/exports"
	"github.com/robertguss/go-islands-starter/internal/server"
	"github.com/robertguss/go-islands-starter/internal/store"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server_failed", "code", err.Error())
		os.Exit(1)
	}
}
func run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	st, err := store.Open(startup, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Pool.Close()
	sessions := auth.Service{Queries: st.Auth, Secure: true}
	oidc, err := auth.NewOIDC(startup, sessions, c.ClientID, c.ClientSecret, c.Origin, c.Domains)
	if err != nil {
		return err
	}
	jobs, err := exports.New(st)
	if err != nil {
		return err
	}
	app := &server.Server{Store: st, Sessions: sessions, OIDC: oidc, Exports: jobs, DestructiveEnabled: c.DestructiveEnabled}
	return server.Run(ctx, ":"+c.Port, app.Handler(), app)
}
