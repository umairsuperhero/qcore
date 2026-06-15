package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qcore-project/qcore/pkg/amf"
	"github.com/qcore-project/qcore/pkg/ausf"
	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/ngap"
	nrfclient "github.com/qcore-project/qcore/pkg/sbi/nrf"
	"github.com/qcore-project/qcore/pkg/sctp"
	"github.com/qcore-project/qcore/pkg/subscriber"
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
		Use:   "qcore-amf",
		Short: "QCore AMF - Access and Mobility Management Function",
		Long:  "QCore AMF implements N2/NGAP (TS 38.413) and NAS 5GMM (TS 24.501) for 5G SA registration.",
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
			fmt.Printf("QCore AMF\n  Version:  %s\n  Commit:   %s\n  Built:    %s\n", version, commit, buildDate)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the AMF server",
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
	log.Info("Starting QCore AMF")

	plmnBytes, err := subscriber.ParsePLMN(cfg.AMF.PLMN)
	if err != nil {
		return fmt.Errorf("parsing AMF PLMN %q: %w", cfg.AMF.PLMN, err)
	}
	plmn := ngap.PLMN(plmnBytes)

	amfCfg := amf.Config{
		NGAPAddr: fmt.Sprintf("%s:%d", cfg.AMF.BindAddress, cfg.AMF.NGAPPort),
		NGAPMode: sctp.Mode(cfg.AMF.SCTPMode),
		GUAMI: ngap.GUAMI{
			PLMN:        plmn,
			AMFRegionID: cfg.AMF.AMFRegionID,
			AMFSetID:    cfg.AMF.AMFSetID,
			AMFPointer:  cfg.AMF.AMFPointer,
		},
		PLMNSupportList: []ngap.PLMNSupportItem{
			{
				PLMN: plmn,
				SNSSAIs: []ngap.SNSSAI{
					{SST: 1}, // eMBB slice
				},
			},
		},
		AMFName:            "QCore-AMF",
		ServingNetworkName: cfg.AMF.ServingNetworkName,
		AMFInstanceID:      cfg.AMF.AMFInstanceID,
		TraceNGAPHex:       os.Getenv("QCORE_AMF_TRACE_NGAP_HEX") == "true",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// NRF: register the AMF and discover peer URLs.
	// Falls back to static config if the NRF is unreachable.
	nrfCli := nrfclient.NewHTTPClient(cfg.AMF.NRFURL, "AMF", false)
	noopEmitter := &events.NoopEmitter{}
	amfProfile := &nrfclient.NFProfile{
		NFInstanceID: cfg.AMF.AMFInstanceID,
		NFType:       nrfclient.NFTypeAMF,
		NFStatus:     nrfclient.StatusRegistered,
		PLMN:         cfg.AMF.PLMN,
		Services: []nrfclient.NFService{{
			ServiceName: "namf-comm",
			Versions:    []string{"v1"},
			Scheme:      "http",
			FQDN:        "amf",
			IPAddr:      cfg.AMF.BindAddress,
			Port:        cfg.AMF.APIPort,
		}},
	}
	lcm := nrfclient.NewLifecycleManager(nrfCli, amfProfile, cfg.AMF.NRFURL, noopEmitter, log)
	go lcm.Start(ctx)

	ausfURL, err := nrfclient.DiscoverFirst(ctx, nrfCli,
		nrfclient.DiscoveryQuery{TargetNFType: nrfclient.NFTypeAUSF, RequesterType: nrfclient.NFTypeAMF},
		cfg.AMF.AUSFURL, noopEmitter, log)
	if err != nil {
		return fmt.Errorf("cannot resolve AUSF: %w", err)
	}

	smfURL, _ := nrfclient.DiscoverFirst(ctx, nrfCli,
		nrfclient.DiscoveryQuery{TargetNFType: nrfclient.NFTypeSMF, RequesterType: nrfclient.NFTypeAMF},
		cfg.AMF.SMFURL, noopEmitter, log)

	amfCfg.SMFURL = smfURL

	ausfCli := ausf.NewClient(ausfURL, "AMF", false)
	amfSvc := amf.NewService(amfCfg, ausfCli, log)
	amfSvc.SetEmitter(events.New(cfg.Telemetry.CollectorURL, "amf", log))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 2)

	// NGAP listener
	go func() {
		log.Infof("AMF NGAP listening on %s (mode=%s)", amfCfg.NGAPAddr, amfCfg.NGAPMode)
		if err := amfSvc.Serve(ctx); err != nil && err != context.Canceled {
			errCh <- fmt.Errorf("AMF NGAP: %w", err)
		}
	}()

	// HTTP health + status API
	apiAddr := fmt.Sprintf("%s:%d", cfg.AMF.BindAddress, cfg.AMF.APIPort)
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "nf": "amf"})
	})
	apiMux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "nf": "amf"})
	})

	apiServer := &http.Server{
		Addr:         apiAddr,
		Handler:      apiMux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	go func() {
		log.Infof("AMF API listening on %s", apiAddr)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("AMF API: %w", err)
		}
	}()

	log.Info("QCore AMF is ready")
	select {
	case sig := <-sigCh:
		log.Infof("Received signal %v, shutting down", sig)
	case err := <-errCh:
		return err
	}

	cancel() // stop NGAP listener
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	return apiServer.Shutdown(shutCtx)
}
