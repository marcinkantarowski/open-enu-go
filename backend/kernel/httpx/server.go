package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Server wraps http.Server with the timeouts a public listener needs and a
// shutdown that lets in-flight requests finish. SSE streams are long-lived,
// so WriteTimeout is deliberately not set here; the SSE handler owns its own
// deadline.
type Server struct {
	Addr    string
	Handler http.Handler
	Log     *slog.Logger

	// ShutdownGrace bounds how long Run waits for in-flight requests once ctx
	// is cancelled. 10s when zero - under Docker's default stop timeout.
	ShutdownGrace time.Duration
}

// Run serves until ctx is cancelled, then drains. It returns nil on a clean
// shutdown and the listen error otherwise.
func (s *Server) Run(ctx context.Context) error {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	grace := s.ShutdownGrace
	if grace == 0 {
		grace = 10 * time.Second
	}
	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           s.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", s.Addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	log.Info("shutting down", "grace", grace.String())
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("shutdown did not finish in time; closing", "error", err)
		_ = srv.Close()
	}
	<-errc
	return nil
}
