package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mccutchen/go-httpbin/v2/httpbin"
)

const (
	maxBodySize = 512 * 1024 // 512kb
	maxDuration = 10 * time.Second
)

var allowedRedirectDomains = []string{
	"example.com",
	"example.net",
	"example.org",
	"httpbingo.org",
}

// Exclude headers set by the fly.io platform on which we're deployed
var excludedHeaders = []string{
	"fly-*",
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	hostname, err := os.Hostname()
	if err != nil {
		logger.Warn("error looking up hostname, using placeholder value", "err", err)
		hostname = "unknown"
	}

	h := httpbin.New(
		httpbin.WithMaxBodySize(maxBodySize),
		httpbin.WithMaxDuration(maxDuration),
		httpbin.WithHostname(hostname),
		httpbin.WithAllowedRedirectDomains(allowedRedirectDomains),
		httpbin.WithExcludeHeaders(strings.Join(excludedHeaders, ",")),
		httpbin.WithObserver(func(ctx context.Context, result httpbin.Result) {
			durationMS := result.Duration.Seconds() * 1000
			lvl := slog.LevelInfo
			if result.Status >= 500 {
				lvl = slog.LevelError
			} else if result.Status >= 400 {
				lvl = slog.LevelWarn
			}
			logger.LogAttrs(ctx, lvl, fmt.Sprintf("%d %s %s %0.0fms", result.Status, result.Method, result.URI, durationMS),
				slog.Int("http.status_code", result.Status),
				slog.String("http.method", result.Method),
				slog.String("http.uri", result.URI),
				slog.Int64("http.response_size_bytes", result.Size),
				slog.Int64("http.request_size_bytes", result.RequestSize),
				slog.String("http.user_agent", result.UserAgent),
				slog.String("network.client_ip", result.ClientIP),
				slog.Float64("duration_ms", durationMS),
			)
		}),
	)

	var handler http.Handler
	handler = h.Handler()
	handler = spamFilter(handler)

	srv := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%s", os.Getenv("PORT")),
		Handler: handler,

		ReadTimeout:       2 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		MaxHeaderBytes:    1024 * 4, // 4kb
	}

	logger.Info("listening on", slog.String("addr", srv.Addr))
	if err := listenAndServeGracefully(srv, maxDuration); err != nil {
		logger.Error("error starting server", "err", err)
		os.Exit(1)
	}
}

// spamFilter is where we attempt to discourage abusive behavior. The actual
// filtering is likely to evolve over time, based on observed behavior and
// traffic patterns.
func spamFilter(next http.Handler) http.Handler {
	isSpam := func(r *http.Request) bool {
		ua := r.Header.Get("User-Agent")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/stream-bytes/500000" && r.URL.Query().Get("nnn") != "":
			// https://github.com/mccutchen/httpbingo.org/issues/1
			return true
		case ua == "Envoy/HC":
			// https://github.com/mccutchen/httpbingo.org/issues/3
			return true
		case ua == "Apache-HttpClient/4.5.14 (Java/21.0.2)" && r.Method == http.MethodGet && r.URL.Path == "/anything":
			// https://github.com/mccutchen/httpbingo.org/issues/4
			return true
		case ua == "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36":
			// https://github.com/mccutchen/httpbingo.org/issues/11
			return true
		case strings.HasPrefix(ua, "cert-manager-challenges/"):
			// https://github.com/mccutchen/httpbingo.org/issues/12
			return true
		case ua == "" && r.URL.Path != "/websocket/echo":
			// https://github.com/mccutchen/httpbingo.org/issues/5
			//
			// this is more aggressive than strictly necessary for the
			// particular traffic pattern in that issue, but it seems
			// reasonable to me to reject all traffic that doesn't include at
			// least *some* User-Agent identifier.
			//
			// [... 6 months later ...]
			//
			// https://github.com/mccutchen/httpbingo.org/issues/8
			//
			// Turns out some websocket clients don't send a User-Agent
			// header, and we'd like to allow them to connect.
			return true
		default:
			return false
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSpam(r) {
			w.WriteHeader(http.StatusPaymentRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
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
