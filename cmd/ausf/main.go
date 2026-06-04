package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/qcore-project/qcore/pkg/ausf"
	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	nrfclient "github.com/qcore-project/qcore/pkg/sbi/nrf"
	"github.com/qcore-project/qcore/pkg/udm"
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
		Use:   "qcore-ausf",
		Short: "QCore AUSF - Authentication Server Function",
		Long:  "QCore AUSF implements Nausf_UEAuthentication (TS 29.509) for 5G-AKA.",
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
			fmt.Printf("QCore AUSF\n  Version:  %s\n  Commit:   %s\n  Built:    %s\n", version, commit, buildDate)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the AUSF server",
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
	log.Info("Starting QCore AUSF")

	udmCli := udm.NewClient(cfg.AUSF.UDMURL, "AUSF", false)
	ausfSvc := ausf.NewService(udmCli, log)

	mux := http.NewServeMux()
	mux.Handle("/", ausfSvc.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	sbiSrv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: cfg.AUSF.BindAddress,
		Port:        cfg.AUSF.Port,
		NFType:      "AUSF",
	}, log, mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// NRF: self-register (non-fatal if NRF is down).
	nrfCli := nrfclient.NewHTTPClient(cfg.AUSF.NRFURL, "AUSF", false)
	ausfProfile := &nrfclient.NFProfile{
		NFInstanceID: "ausf-" + cfg.AUSF.BindAddress,
		NFType:       nrfclient.NFTypeAUSF,
		NFStatus:     nrfclient.StatusRegistered,
		Services: []nrfclient.NFService{{
			ServiceName: "nausf-auth",
			Versions:    []string{"v1"},
			Scheme:      "http",
			FQDN:        "ausf",
			IPAddr:      cfg.AUSF.BindAddress,
			Port:        cfg.AUSF.Port,
		}},
	}
	go nrfclient.NewLifecycleManager(nrfCli, ausfProfile, cfg.AUSF.NRFURL, &events.NoopEmitter{}, log).Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		log.Infof("AUSF SBI listening on %s:%d", cfg.AUSF.BindAddress, cfg.AUSF.Port)
		if err := sbiSrv.Serve(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("AUSF SBI: %w", err)
		}
	}()

	log.Info("QCore AUSF is ready")
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
