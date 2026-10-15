package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

func Run(ctx context.Context, address string, handler http.Handler, s *Server) error {
	if err := s.Exports.Queue.Start(ctx); err != nil {
		return errors.New("worker startup failed")
	}
	httpServer := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	result := make(chan error, 1)
	go func() { result <- httpServer.ListenAndServe() }()
	slog.Info("server_ready", "address", address)
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-result:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	httpErr := httpServer.Shutdown(shutdown)
	workerErr := s.Exports.Queue.Stop(shutdown)
	if workerErr != nil {
		force, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Exports.Queue.StopAndCancel(force)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return errors.New("server stopped unexpectedly")
	}
	if httpErr != nil || workerErr != nil {
		return errors.New("shutdown deadline exceeded")
	}
	slog.Info("server_stopped")
	return nil
}
