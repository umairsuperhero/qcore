package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/dashboard"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
	cfgFile   string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "qcore-dashboard",
		Short: "QCore Dashboard - operator BFF and web UI",
		Long: "QCore Dashboard is the single process the browser talks to. " +
			"It aggregates state across all network functions, proxies the live " +
			"event stream from the collector, controls the built-in simulator, " +
			"and serves the web UI.",
	}
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default: config.yaml)")
	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("QCore Dashboard\n  Version:  %s\n  Commit:   %s\n  Built:    %s\n", version, commit, buildDate)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the dashboard server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer()
		},
	}
}

func runServer() error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	log := logger.New(cfg.Logging.Level, cfg.Logging.Format)
	log.Info("Starting QCore Dashboard")

	server, err := dashboard.New(cfg, log)
	if err != nil {
		return fmt.Errorf("constructing dashboard: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Dashboard.BindAddress, cfg.Dashboard.Port)
	httpServer := &http.Server{
		Addr:        addr,
		Handler:     server.Handler(),
		ReadTimeout: 15 * time.Second,
		// WriteTimeout is intentionally 0 — the SSE proxy holds connections
		// open for the lifetime of the browser tab.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Infof("Dashboard listening on http://%s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("dashboard server: %w", err)
		}
	}()

	log.Infof("QCore Dashboard is ready (open http://%s)", addr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Infof("Received signal %v, shutting down", sig)
	case err := <-errCh:
		return err
	}

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = httpServer.Shutdown(shutCtx)
	log.Info("QCore Dashboard stopped")
	return nil
}
