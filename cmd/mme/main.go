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
	"github.com/qcore-project/qcore/pkg/hss"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
	"github.com/qcore-project/qcore/pkg/mme"
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
		Use:   "qcore-mme",
		Short: "QCore MME - Mobility Management Entity",
		Long:  "QCore MME handles S1AP signaling from eNodeBs, authenticates UEs via the HSS, and manages EPS bearers.",
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default: config.yaml)")

	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(testCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("QCore MME\n  Version:  %s\n  Commit:   %s\n  Built:    %s\n", version, commit, buildDate)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the MME server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer()
		},
	}
}

func runServer() error {
	// Load config
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Initialize logger
	log := logger.New(cfg.Logging.Level, cfg.Logging.Format)
	log.Info("Starting QCore MME")

	// Parse PLMN
	plmnID, err := hss.ParsePLMN(cfg.MME.PLMN)
	if err != nil {
		return fmt.Errorf("parsing PLMN %q: %w (example: 00101 for test network)", cfg.MME.PLMN, err)
	}

	// Initialize metrics
	m := metrics.New()
	mmeMetrics := metrics.RegisterMMEMetrics(m)

	// Create S6a client (HTTP to HSS)
	s6a := mme.NewS6aClient(cfg.MME.HSSURL, log)

	// Check HSS connectivity
	if err := s6a.HealthCheck(); err != nil {
		log.Warnf("HSS not reachable at %s: %v (MME will retry on auth requests)", cfg.MME.HSSURL, err)
	} else {
		log.Infof("HSS connected at %s", cfg.MME.HSSURL)
	}

	// Create MME
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mmeService := mme.New(&cfg.MME, plmnID, log, mmeMetrics, s6a)

	// Start S1AP listener
	if err := mmeService.Start(ctx); err != nil {
		return fmt.Errorf("starting MME: %w", err)
	}
	defer mmeService.Stop()

	// Start debug/status API
	api := mme.NewAPI(mmeService, log, mmeMetrics)
	apiAddr := fmt.Sprintf("%s:%d", cfg.MME.BindAddress, cfg.MME.APIPort)
	apiServer := &http.Server{
		Addr:         apiAddr,
		Handler:      api.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 2)

	go func() {
		log.Infof("MME API listening on %s", apiAddr)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("API server: %w", err)
		}
	}()

	// Start metrics server
	if cfg.Metrics.Enabled {
		metricsAddr := fmt.Sprintf("%s:%d", cfg.MME.BindAddress, cfg.Metrics.Port)
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", m.Handler())
		metricsServer := &http.Server{
			Addr:    metricsAddr,
			Handler: metricsMux,
		}
		go func() {
			log.Infof("Metrics server listening on %s", metricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("metrics server: %w", err)
			}
		}()

		defer func() {
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutCancel()
			metricsServer.Shutdown(shutCtx)
		}()
	}

	log.Infof("QCore MME is ready (PLMN=%s, S1AP=%s:%d, mode=%s)",
		cfg.MME.PLMN, cfg.MME.BindAddress, cfg.MME.S1APPort, cfg.MME.SCTPMode)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Infof("Received signal %v, shutting down", sig)
	case err := <-errCh:
		return err
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	cancel() // stop accept loop

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down API server: %w", err)
	}

	log.Info("QCore MME stopped")
	return nil
}
