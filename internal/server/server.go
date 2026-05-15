package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Server wraps http.Server with graceful-shutdown behaviour.
type Server struct {
	httpServer *http.Server
}

// New builds a Server listening on the given port.
func New(port string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              ":" + port,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Run starts the server and blocks until ctx is cancelled or the server fails.
// On cancellation it shuts down gracefully with a 10-second timeout.
func (s *Server) Run(ctx context.Context, log *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("server starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	}
}
