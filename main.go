package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mccutchen/go-httpbin/v2/httpbin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

const (
	maxBodySize = 512 * 1024 // 512kb
	maxDuration = 10 * time.Second
)

func main() {
	logger := zerolog.New(os.Stderr)

	hostname, err := os.Hostname()
	if err != nil {
		logger.Warn().Msgf("error looking up hostname: %s", err)
		hostname = "unknown"
	}

	h := httpbin.New(
		httpbin.WithMaxBodySize(maxBodySize),
		httpbin.WithMaxDuration(maxDuration),
		httpbin.WithHostname(hostname),
		httpbin.WithAllowedRedirectDomains([]string{
			"httpbingo.org",
			"example.org",
		}),
	)

	var handler http.Handler
	handler = h.Handler()
	handler = hlog.AccessHandler(requestLogger)(handler)
	handler = hlog.NewHandler(logger)(handler)

	srv := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%s", os.Getenv("PORT")),
		Handler: handler,
	}

	logger.Info().Msgf("listening on %s", srv.Addr)
	if err := listenAndServeGracefully(srv, maxDuration); err != nil {
		logger.Fatal().Msgf("error starting server: %s", err)
	}
}

func requestLogger(r *http.Request, status int, size int, duration time.Duration) {
	hlog.FromRequest(r).
		Info().
		Int("status", status).
		Str("method", r.Method).
		Str("uri", r.RequestURI).
		Int("size_bytes", size).
		Str("user_agent", r.Header.Get("User-Agent")).
		Str("client_ip", r.Header.Get("Fly-Client-IP")).
		Float64("duration_ms", duration.Seconds()*1e3). // https://github.com/golang/go/issues/5491#issuecomment-66079585
		Send()
}

func listenAndServeGracefully(srv *http.Server, shutdownTimeout time.Duration) error {
	doneCh := make(chan error, 1)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		doneCh <- srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}

	return <-doneCh
}
