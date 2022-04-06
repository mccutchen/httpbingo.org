package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

	h := httpbin.New(
		httpbin.WithMaxBodySize(maxBodySize),
		httpbin.WithMaxDuration(maxDuration),
	)

	var handler http.Handler
	handler = h.Handler()
	handler = trafficController(handler)
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

func trafficController(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Special case requests from Cloudflare-Traffic-Manager, which is
		// hammering httpbingo.org for unknown reasons. More context in this
		// community support request:
		// https://community.cloudflare.com/t/unexpected-excessive-requests-from-cloudflare-traffic-manager-to-non-cloudflare-domain/374760
		if strings.Contains(r.Header.Get("User-Agent"), "Cloudflare-Traffic-Manager") {
			http.Error(w, "Fuck off CloudFlare", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
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
