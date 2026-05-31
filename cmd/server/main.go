package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nagayon-935/conduit/internal/api"
	"github.com/nagayon-935/conduit/internal/config"
	"github.com/nagayon-935/conduit/internal/connlog"
	"github.com/nagayon-935/conduit/internal/session"
	"github.com/nagayon-935/conduit/internal/sshconn"
	"github.com/nagayon-935/conduit/internal/vault"
)

const (
	httpReadTimeout  = 30 * time.Second
	httpIdleTimeout  = 120 * time.Second
	shutdownTimeout  = 30 * time.Second
	connLogStoreSize = 200
)

func main() {
	// Configure structured logging.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

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

	var logStore connlog.Store
	if cfg.DBPath != "" {
		sqliteStore, err := connlog.NewSQLiteStore(cfg.DBPath, connLogStoreSize)
		if err != nil {
			slog.Error("failed to open sqlite store", "error", err)
			os.Exit(1)
		}
		defer sqliteStore.Close()
		logStore = sqliteStore
		slog.Info("using sqlite log store", "path", cfg.DBPath)
	} else {
		logStore = connlog.NewMemoryStore(connLogStoreSize)
		slog.Info("using in-memory log store (set DB_PATH for persistence)")
	}

	// Step 3: Start session garbage collector.
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	sessionManager.StartGC(rootCtx)
	slog.Info("session GC started", "interval", cfg.SessionGCInterval)

	// Step 4: Wire routes.
	handler := api.NewHandler(cfg, sessionManager, vaultClient, dialer, logStore)
	routes := handler.Routes()

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
