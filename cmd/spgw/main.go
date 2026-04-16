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
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
	"github.com/qcore-project/qcore/pkg/spgw"
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
		Use:   "qcore-spgw",
		Short: "QCore SPGW - collapsed Serving + PDN gateway",
		Long:  "QCore SPGW terminates the GTP-U tunnel from the eNodeB, allocates UE IPs, and forwards user-plane packets to an egress (log / TUN).",
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
			fmt.Printf("QCore SPGW\n  Version:  %s\n  Commit:   %s\n  Built:    %s\n", version, commit, buildDate)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the SPGW service",
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
	log.Info("Starting QCore SPGW")

	svc, err := spgw.New(&cfg.SPGW, log)
	if err != nil {
		return fmt.Errorf("constructing SPGW: %w", err)
	}

	// Build metrics up-front so the dataplane can pick them up at Start().
	var spgwMetrics *metrics.SPGWMetrics
	var metricsHandler http.Handler
	if cfg.Metrics.Enabled {
		m := metrics.New()
		spgwMetrics = metrics.RegisterSPGWMetrics(m)
		metricsHandler = m.Handler()
		svc.SetMetrics(spgwMetrics)
	}

	if err := svc.Start(); err != nil {
		return fmt.Errorf("starting SPGW: %w", err)
	}
	defer svc.Stop()

	api := spgw.NewAPI(svc)
	if spgwMetrics != nil {
		api.SetMetrics(spgwMetrics)
	}
	apiAddr := fmt.Sprintf("%s:%d", cfg.SPGW.BindAddress, cfg.SPGW.APIPort)
	apiServer := &http.Server{
		Addr:         apiAddr,
		Handler:      api.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Infof("SPGW API listening on %s", apiAddr)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("API server: %w", err)
		}
	}()

	// Metrics HTTP endpoint (separate port so /metrics is firewall-isolatable).
	if metricsHandler != nil {
		metricsAddr := fmt.Sprintf("%s:%d", cfg.SPGW.BindAddress, cfg.Metrics.Port)
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", metricsHandler)
		metricsServer := &http.Server{Addr: metricsAddr, Handler: metricsMux}
		go func() {
			log.Infof("Metrics server listening on %s", metricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("metrics server: %w", err)
			}
		}()
		defer func() {
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutCancel()
			_ = metricsServer.Shutdown(shutCtx)
		}()
	}

	log.Infof("QCore SPGW is ready (API=%s, S1-U=:%d, pool=%s, egress=%s)",
		apiAddr, cfg.SPGW.S1UPort, cfg.SPGW.UEPool, cfg.SPGW.Egress)

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
	_ = apiServer.Shutdown(shutCtx)
	log.Info("QCore SPGW stopped")
	return nil
}
