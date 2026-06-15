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
	"github.com/qcore-project/qcore/pkg/suci"
	"github.com/qcore-project/qcore/pkg/udm"
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
		Use:   "qcore-udm",
		Short: "QCore UDM - Unified Data Management",
		Long:  "QCore UDM exposes Nudm_SDM and Nudm_UEAU (TS 29.503) for 5G subscriber data and authentication.",
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
			fmt.Printf("QCore UDM\n  Version:  %s\n  Commit:   %s\n  Built:    %s\n", version, commit, buildDate)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the UDM server",
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
	log.Info("Starting QCore UDM")

	var udmSvc *udm.Service
	sidfResolver, err := buildSIDFResolver(cfg.UDM.SIDFKeys)
	if err != nil {
		return err
	}

	if cfg.UDM.UDRURL != "" {
		// Network mode: fetch subscriber data from UDR over SBI.
		udrCli := udr.NewClient(cfg.UDM.UDRURL, "UDM", false)
		amSrc := udm.NewUDRSource(udrCli, cfg.UDM.PLMN)
		authSrc := udm.NewUDRAuthSource(udrCli)
		udmSvc = udm.NewService(amSrc, log).WithSIDFResolver(sidfResolver).WithAuthSource(authSrc)
		log.Infof("UDM using UDR backend at %s", cfg.UDM.UDRURL)
	} else {
		// Direct mode: read subscriber database directly (dev/single-node).
		db, err := database.Connect(&cfg.Database, log)
		if err != nil {
			return fmt.Errorf("connecting to database: %w", err)
		}
		if err := database.AutoMigrate(db, &subscriber.Subscriber{}); err != nil {
			return fmt.Errorf("running migrations: %w", err)
		}
		subsvc := subscriber.NewService(db, log, nil, [3]byte{})
		if err := resetDemoSubscriberIfRequested(context.Background(), subsvc, log); err != nil {
			log.Warnf("Could not reset demo subscriber (not fatal): %v", err)
		}
		amSrc := udm.NewStoreSource(subsvc)
		authSrc := udm.NewStoreAuthSource(subsvc)
		udmSvc = udm.NewService(amSrc, log).WithSIDFResolver(sidfResolver).WithAuthSource(authSrc)
		log.Info("UDM using direct database backend")
	}
	udmSvc.SetEmitter(events.New(cfg.Telemetry.CollectorURL, "udm", log))

	mux := http.NewServeMux()
	mux.Handle("/", udmSvc.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	sbiSrv := sbi.NewServer(sbi.ServerConfig{
		BindAddress: cfg.UDM.BindAddress,
		Port:        cfg.UDM.Port,
		NFType:      "UDM",
	}, log, mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// NRF: self-register (non-fatal if NRF is down).
	nrfCli := nrfclient.NewHTTPClient(cfg.UDM.NRFURL, "UDM", false)
	udmProfile := &nrfclient.NFProfile{
		NFInstanceID: "udm-" + cfg.UDM.BindAddress,
		NFType:       nrfclient.NFTypeUDM,
		NFStatus:     nrfclient.StatusRegistered,
		Services: []nrfclient.NFService{{
			ServiceName: "nudm-sdm",
			Versions:    []string{"v1"},
			Scheme:      "http",
			FQDN:        "udm",
			IPAddr:      cfg.UDM.BindAddress,
			Port:        cfg.UDM.Port,
		}},
	}
	go nrfclient.NewLifecycleManager(nrfCli, udmProfile, cfg.UDM.NRFURL, &events.NoopEmitter{}, log).Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		log.Infof("UDM SBI listening on %s:%d", cfg.UDM.BindAddress, cfg.UDM.Port)
		if err := sbiSrv.Serve(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("UDM SBI: %w", err)
		}
	}()

	log.Info("QCore UDM is ready")
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

func resetDemoSubscriberIfRequested(ctx context.Context, service *subscriber.Service, log logger.Logger) error {
	if os.Getenv("QCORE_RESET_DEMO_SUBSCRIBER") != "true" {
		return nil
	}

	demo := &subscriber.Subscriber{
		IMSI:   "001010000000001",
		Ki:     "465b5ce8b199b49faa5f0a2ee238a6bc",
		OPc:    "cd63cb71954a9f4e48a5994e37a02baf",
		AMF:    "8000",
		SQN:    "000000000020",
		APN:    "internet",
		Status: 0,
	}
	if err := service.UpdateSubscriber(ctx, demo.IMSI, demo); err != nil {
		if createErr := service.CreateSubscriber(ctx, demo); createErr != nil {
			return fmt.Errorf("update demo subscriber: %v; create demo subscriber: %w", err, createErr)
		}
		log.Infof("Created demo subscriber (IMSI=%s) for UDM direct-mode auth.", demo.IMSI)
		return nil
	}
	log.Infof("Reset demo subscriber (IMSI=%s) for UDM direct-mode auth.", demo.IMSI)
	return nil
}

func buildSIDFResolver(cfgKeys []config.SIDFKeyConfig) (*suci.Resolver, error) {
	keys := make([]suci.HomeNetworkPrivateKey, 0, len(cfgKeys))
	for _, cfgKey := range cfgKeys {
		priv, err := suci.ParsePrivateKeyHex(cfgKey.Scheme, cfgKey.PrivateKeyHex)
		if err != nil {
			return nil, fmt.Errorf("loading UDM SIDF key id %d scheme %d: %w", cfgKey.ID, cfgKey.Scheme, err)
		}
		keys = append(keys, suci.HomeNetworkPrivateKey{
			ID:         cfgKey.ID,
			Scheme:     cfgKey.Scheme,
			PrivateKey: priv,
		})
	}
	resolver, err := suci.NewResolver(keys)
	if err != nil {
		return nil, fmt.Errorf("building UDM SIDF resolver: %w", err)
	}
	return resolver, nil
}
