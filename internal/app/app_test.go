package app

import (
	"bytes"
	"context"
	"errors"
	"io"

	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mananuf/justbarme/internal/config"
)

type fakeServer struct {
	listenErr        error
	shutdownErr      error
	stopped          chan struct{}
	stopOnce         sync.Once
	shutdownCalled   bool
	shutdownDeadline bool
}

func newFakeServer() *fakeServer {
	return &fakeServer{stopped: make(chan struct{})}
}

func (s *fakeServer) ListenAndServe() error {
	if s.listenErr != nil {
		return s.listenErr
	}
	<-s.stopped
	return http.ErrServerClosed
}

func (s *fakeServer) Shutdown(ctx context.Context) error {
	s.shutdownCalled = true
	_, s.shutdownDeadline = ctx.Deadline()
	s.stopOnce.Do(func() { close(s.stopped) })
	return s.shutdownErr
}

func TestRunServerShutsDownOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := newFakeServer()

	if err := runServer(ctx, server, time.Second); err != nil {
		t.Fatalf("runServer() error = %v", err)
	}
	if !server.shutdownCalled || !server.shutdownDeadline {
		t.Fatalf("shutdown called=%v deadline=%v", server.shutdownCalled, server.shutdownDeadline)
	}
}

func TestRunServerReturnsListenError(t *testing.T) {
	server := newFakeServer()
	server.listenErr = errors.New("bind failed")

	err := runServer(context.Background(), server, time.Second)
	if err == nil || !strings.Contains(err.Error(), "bind failed") {
		t.Fatalf("runServer() error = %v", err)
	}
}

func TestRunServerReturnsShutdownError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := newFakeServer()
	server.shutdownErr = errors.New("shutdown failed")

	err := runServer(ctx, server, time.Second)
	if err == nil || !strings.Contains(err.Error(), "shutdown failed") {
		t.Fatalf("runServer() error = %v", err)
	}
}

func TestNewLoggerHonorsLevel(t *testing.T) {
	var output bytes.Buffer
	logger := newLogger(config.Config{LogLevel: "warn"}, &output)
	logger.Info("hidden")
	logger.Warn("visible")

	if strings.Contains(output.String(), "hidden") || !strings.Contains(output.String(), "visible") {
		t.Fatalf("unexpected logger output: %s", output.String())
	}
}

func BenchmarkNewLogger(b *testing.B) {
	cfg := config.Config{LogLevel: "info"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = newLogger(cfg, io.Discard)
	}
}
