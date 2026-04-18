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
	"github.com/qcore-project/qcore/pkg/database"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/qcore-project/qcore/pkg/subscriber/admin"
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
		Use:   "qcore-hss",
		Short: "QCore HSS - Home Subscriber Server",
		Long:  "QCore HSS manages subscriber data and generates authentication vectors for the LTE core network.",
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default: config.yaml)")

	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(subscriberCmd())
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
			fmt.Printf("QCore HSS\n  Version:  %s\n  Commit:   %s\n  Built:    %s\n", version, commit, buildDate)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the HSS server",
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
	log.Info("Starting QCore HSS")

	// Connect to database
	db, err := database.Connect(&cfg.Database, log)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	log.Info("Running database migrations")
	if err := database.AutoMigrate(db, &subscriber.Subscriber{}); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Initialize metrics
	m := metrics.New()
	hssMetrics := metrics.RegisterHSSMetrics(m)

	// Parse PLMN
	plmnID, err := subscriber.ParsePLMN("00101")
	if err != nil {
		return fmt.Errorf("parsing PLMN: %w", err)
	}

	// Create subscriber service + admin API
	service := subscriber.NewService(db, log, hssMetrics, plmnID)
	api := admin.NewAPI(service, db, log, hssMetrics)

	// Zero-config delight: seed a demo subscriber on first run so curl works immediately.
	// Uses 3GPP TS 35.208 Test Set 1 — the canonical Milenage test credentials.
	if err := seedDemoSubscriberIfEmpty(context.Background(), service, log); err != nil {
		log.Warnf("Could not seed demo subscriber (not fatal): %v", err)
	}

	// Start API server
	apiAddr := fmt.Sprintf("%s:%d", cfg.HSS.BindAddress, cfg.HSS.APIPort)
	apiServer := &http.Server{
		Addr:         apiAddr,
		Handler:      api.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 2)

	go func() {
		log.Infof("HSS API listening on %s", apiAddr)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("API server: %w", err)
		}
	}()

	// Start metrics server
	if cfg.Metrics.Enabled {
		metricsAddr := fmt.Sprintf("%s:%d", cfg.HSS.BindAddress, cfg.Metrics.Port)
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
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			metricsServer.Shutdown(ctx)
		}()
	}

	log.Info("QCore HSS is ready")

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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutting down API server: %w", err)
	}

	log.Info("QCore HSS stopped")
	return nil
}

// seedDemoSubscriberIfEmpty creates a single demo subscriber when the database
// is empty, so new users can hit the API with zero setup. The IMSI and keys
// come from 3GPP TS 35.208 Test Set 1 — verified test vectors, not secrets.
func seedDemoSubscriberIfEmpty(ctx context.Context, service *subscriber.Service, log logger.Logger) error {
	if os.Getenv("QCORE_SKIP_SEED") == "true" {
		return nil
	}

	_, total, err := service.ListSubscribers(ctx, 1, 1, "")
	if err != nil {
		return fmt.Errorf("checking subscriber count: %w", err)
	}
	if total > 0 {
		return nil // don't touch existing data
	}

	demo := &subscriber.Subscriber{
		IMSI:   "001010000000001",
		Ki:     "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:    "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:    "8000",
		SQN:    "000000000000",
		APN:    "internet",
		Status: 0, // active
	}
	if err := service.CreateSubscriber(ctx, demo); err != nil {
		return err
	}

	log.Infof("🎉 Seeded demo subscriber (IMSI=%s). Try:", demo.IMSI)
	log.Infof("   curl -X POST http://localhost:8080/api/v1/subscribers/%s/auth-vector", demo.IMSI)
	log.Infof("   (disable with QCORE_SKIP_SEED=true)")
	return nil
}
