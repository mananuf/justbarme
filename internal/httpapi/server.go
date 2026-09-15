package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/mananuf/justbarme/internal/config"
)

type Server struct {
	httpServer *http.Server
}

func NewServer(cfg config.HTTP, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{httpServer: &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}}
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
