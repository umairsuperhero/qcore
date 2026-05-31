package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/database"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	nrfclient "github.com/qcore-project/qcore/pkg/sbi/nrf"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"github.com/qcore-project/qcore/pkg/udr"
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
		Use:   "qcore-udr",
		Short: "QCore UDR - Unified Data Repository",
		Long:  "QCore UDR exposes Nudr_DataRepository (TS 29.504 / 29.505) backed by the subscriber database.",
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
			fmt.Printf("QCore UDR\n  Version:  %s\n  Commit:   %s\n  Built:    %s\n", version, commit, buildDate)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the UDR server",
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
	log.Info("Starting QCore UDR")

	db, err := database.Connect(&cfg.Database, log)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	if err := database.AutoMigrate(db, &subscriber.Subscriber{}); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	subsvc := subscriber.NewService(db, log, nil, [3]byte{})
	udrSvc := udr.NewService(subsvc, log)

	if err := seedDemoSubscriberIfEmpty(context.Background(), subsvc, log); err != nil {
		log.Warnf("Could not seed demo subscriber (not fatal): %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", udrSvc.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	sbiSrv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: cfg.UDR.BindAddress,
		Port:        cfg.UDR.Port,
		NFType:      "UDR",
	}, log, mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// NRF: self-register (non-fatal if NRF is down).
	nrfCli := nrfclient.NewHTTPClient(cfg.UDR.NRFURL, "UDR", false)
	udrProfile := &nrfclient.NFProfile{
		NFInstanceID: "udr-" + cfg.UDR.BindAddress,
		NFType:       nrfclient.NFTypeUDR,
		NFStatus:     nrfclient.StatusRegistered,
		Services: []nrfclient.NFService{{
			ServiceName: "nudr-dr",
			Versions:    []string{"v1"},
			Scheme:      "http",
			IPAddr:      cfg.UDR.BindAddress,
			Port:        cfg.UDR.Port,
		}},
	}
	go nrfclient.NewLifecycleManager(nrfCli, udrProfile, cfg.UDR.NRFURL, &events.NoopEmitter{}, log).Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		log.Infof("UDR SBI listening on %s:%d", cfg.UDR.BindAddress, cfg.UDR.Port)
		if err := sbiSrv.Serve(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("UDR SBI: %w", err)
		}
	}()

	log.Info("QCore UDR is ready")
	select {
	case sig := <-sigCh:
		log.Infof("Received signal %v, shutting down", sig)
	case err := <-errCh:
		return err
	}

	shutCtx, shutCancel := context.WithCancel(ctx)
	defer shutCancel()
	return sbiSrv.Shutdown(shutCtx)
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

	log.Infof("🎉 Seeded demo subscriber (IMSI=%s) in UDR.", demo.IMSI)
	return nil
}
