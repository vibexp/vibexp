package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"

	"github.com/vibexp/vibexp/internal/config"
)

// PprofServer is the opt-in profiling listener (#1277): net/http/pprof on its
// own http.Server and its own address, never on the API port. It carries no
// authentication — it is protected by where it binds (loopback by default) —
// so it must not be mounted on the main router.
type PprofServer struct {
	srv      *http.Server
	listener net.Listener
	logger   *slog.Logger
}

// newPprofMux mounts the pprof handlers on a dedicated mux. net/http/pprof also
// registers itself on http.DefaultServeMux when imported, but nothing in this
// process serves the default mux, so these routes are the only way in.
func newPprofMux() *http.ServeMux {
	mux := http.NewServeMux()
	// Index also serves the named profiles (heap, goroutine, allocs, block,
	// mutex, threadcreate) under /debug/pprof/<name>.
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

// StartPprofServer binds cfg.ListenAddr and serves pprof on it in a background
// goroutine. It does not read cfg.Enabled: the caller decides whether to start
// it at all (cmd.startPprof), so with the default config it is never called and
// nothing listens. The caller owns the returned server and must Shutdown it.
func StartPprofServer(ctx context.Context, cfg config.PprofConfig, logger *slog.Logger) (*PprofServer, error) {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind pprof listener on %s: %w", cfg.ListenAddr, err)
	}

	p := &PprofServer{
		srv: &http.Server{
			Handler: newPprofMux(),
			// No WriteTimeout: /debug/pprof/profile and /trace stream for the
			// requested number of seconds and refuse a duration longer than it.
			ReadHeaderTimeout: serverReadHeaderTimeout,
			IdleTimeout:       serverIdleTimeout,
		},
		listener: listener,
		logger:   logger,
	}

	if !isLoopbackListenAddr(listener.Addr()) {
		logger.Warn("pprof listener is bound to a non-loopback address and has no authentication: "+
			"profiles expose heap contents and can be used to load the process. Make sure the port is "+
			"reachable only from trusted hosts (e.g. published to the host's loopback)",
			"listen_addr", p.Addr())
	}
	logger.Info("pprof listener started", "listen_addr", p.Addr())

	go func() {
		if err := p.srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("pprof listener failed", "error", err)
		}
	}()

	return p, nil
}

// Addr returns the address the listener is bound to (the resolved port when
// the configured one was 0).
func (p *PprofServer) Addr() string {
	return p.listener.Addr().String()
}

// Shutdown stops the listener, waiting for in-flight profile requests until ctx
// is done. It is safe on a nil server (profiling disabled or failed to bind).
func (p *PprofServer) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.srv.Shutdown(ctx)
}

// isLoopbackListenAddr reports whether a bound listener address is loopback
// only. An unspecified address (0.0.0.0, ::) listens on every interface, so it
// is not.
func isLoopbackListenAddr(addr net.Addr) bool {
	tcpAddr, ok := addr.(*net.TCPAddr)
	return ok && tcpAddr.IP.IsLoopback()
}
