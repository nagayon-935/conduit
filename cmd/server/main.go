package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nagayon-935/conduit/internal/api"
	"github.com/nagayon-935/conduit/internal/config"
	"github.com/nagayon-935/conduit/internal/control"
	"github.com/nagayon-935/conduit/internal/session"
	"github.com/nagayon-935/conduit/internal/sshconn"
	"github.com/nagayon-935/conduit/internal/vault"
)

const (
	httpReadTimeout = 30 * time.Second
	httpIdleTimeout = 120 * time.Second
	shutdownTimeout = 30 * time.Second
)

func main() {
	// Configure structured logging.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if len(os.Args) > 1 {
		if err := offline(os.Args[1:]); err != nil {
			slog.Error("offline command failed", "error", err)
			os.Exit(1)
		}
		return
	}
	// Step 1: Load configuration from environment.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("configuration loaded",
		"server_addr", cfg.ServerAddr,
		"vault_addr", cfg.VaultAddr,
		"vault_ssh_mount", cfg.VaultSSHMount,
		"vault_ssh_role", cfg.VaultSSHRole,
		"grace_period", cfg.GracePeriod,
		"gc_interval", cfg.SessionGCInterval,
	)

	// Step 2: Build application dependencies.
	vaultClient, err := vault.NewClient(cfg.VaultAddr, cfg.VaultToken.Value(), cfg.VaultSSHMount, cfg.VaultSSHRole)
	if err != nil {
		slog.Error("failed to create vault client", "error", err)
		os.Exit(1)
	}

	dialer := sshconn.NewDialer(cfg.KnownHostsPath)
	sessionManager := session.NewManager(cfg)

	if !cfg.DevHTTP && cfg.KnownHostsPath == "" {
		slog.Error("KNOWN_HOSTS_PATH is required (except explicit CONDUIT_DEV_HTTP=true)")
		os.Exit(1)
	}
	if cfg.PublicURL != "" {
		u, e := url.Parse(cfg.PublicURL)
		if e != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (!cfg.DevHTTP && u.Scheme != "https") || (u.Scheme != "http" && u.Scheme != "https") {
			slog.Error("PUBLIC_URL must be an HTTPS origin")
			os.Exit(1)
		}
	}
	for _, c := range append(append([]string{}, cfg.AllowedCIDRs...), cfg.TrustedProxyCIDRs...) {
		if _, e := netip.ParsePrefix(c); e != nil {
			slog.Error("invalid SSH_ALLOWED_CIDRS entry", "cidr", c)
			os.Exit(1)
		}
	}
	store, err := control.Open(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open identity database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Step 3: Start session garbage collector.
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	sessionManager.StartGC(rootCtx)
	slog.Info("session GC started", "interval", cfg.SessionGCInterval)

	// Step 4: Wire routes.
	handler := api.NewHandler(cfg, sessionManager, vaultClient, dialer, store)
	routes := handler.Routes()
	handler.StartMaintenance(rootCtx)
	defer handler.Close()

	srv := &http.Server{
		Addr:         cfg.ServerAddr,
		Handler:      routes,
		ReadTimeout:  httpReadTimeout,
		WriteTimeout: 0, // 0 = no timeout on writes (WebSocket connections are long-lived)
		IdleTimeout:  httpIdleTimeout,
	}

	// Step 5: Serve with graceful shutdown on SIGTERM / SIGINT.
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGTERM, syscall.SIGINT)

	serverErrCh := make(chan error, 1)
	go func() {
		slog.Info("HTTP server starting", "addr", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	select {
	case sig := <-shutdownCh:
		slog.Info("shutdown signal received", "signal", sig)
	case err := <-serverErrCh:
		slog.Error("server error", "error", err)
		rootCancel()
		os.Exit(1)
	}

	// Cancel the root context to stop GC and any background work.
	rootCancel()

	// Give in-flight requests up to the configured timeout to complete.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("server shut down cleanly")
}
