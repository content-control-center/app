package server

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on http.DefaultServeMux
	"time"

	"github.com/ogen-app/ogen/src/logging"
)

// startPprof serves net/http/pprof on a container-internal address (default
// localhost:6060, reachable via `docker compose exec`). Opt-in via ENABLE_PPROF.
//
// The point is the goroutine dump: during a slow request run
//
//	curl -s 'localhost:6060/debug/pprof/goroutine?debug=2'
//
// and read what every goroutine is doing — a read-loop parked in
// runtime_pollWait means the bytes genuinely haven't arrived (network/API),
// whereas a spinning goroutine or a runaway goroutine count means we're
// starving ourselves. Diagnostic only; keep off in production.
func startPprof(addr string) {
	if addr == "" {
		addr = "localhost:6060"
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		slog.Info("pprof server listening", logging.AttrComponent, "pprof", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("pprof server stopped", logging.AttrComponent, "pprof", logging.AttrError, err)
		}
	}()
}
