package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/sbi"
	nrfclient "github.com/qcore-project/qcore/pkg/sbi/nrf"
	"github.com/qcore-project/qcore/pkg/smf"
)

func main() {
	var cfgFile string

	rootCmd := &cobra.Command{
		Use:   "smf",
		Short: "QCore Session Management Function",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./smf.yaml)")

	viper.SetDefault("sbi.bindAddress", "0.0.0.0")
	viper.SetDefault("sbi.advertiseAddress", "")
	viper.SetDefault("sbi.port", 8002) // AMF=8000, AUSF=8006, UDM=8003, UDR=8004, NRF=8001
	viper.SetDefault("sbi.nrfUrl", "http://nrf:8001")
	viper.SetDefault("smf.ipPoolCidr", "10.45.0.0/16")
	viper.SetDefault("smf.upfPfcpAddr", "upf:8805")
	viper.SetDefault("smf.localPfcpIp", "0.0.0.0")

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	log := logger.New("debug", "text")
	log.Info("Starting SMF")

	instanceID := uuid.New().String()

	smfCfg := smf.Config{
		IPPoolCIDR:    viper.GetString("smf.ipPoolCidr"),
		UPFPfcpAddr:   viper.GetString("smf.upfPfcpAddr"),
		LocalPfcpIP:   viper.GetString("smf.localPfcpIp"),
		SMFInstanceID: instanceID,
	}

	svc, err := smf.NewService(smfCfg, log)
	if err != nil {
		return fmt.Errorf("failed to create SMF service: %w", err)
	}
	defer svc.Close()

	srvCfg := sbi.ServerConfig{
		BindAddress: viper.GetString("sbi.bindAddress"),
		Port:        viper.GetInt("sbi.port"),
		NFType:      "SMF",
	}

	srv := sbi.NewServer(srvCfg, log, svc.Handler())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background SMF tasks (e.g. PFCP Association)
	if err := svc.Start(ctx); err != nil {
		return fmt.Errorf("failed to start SMF tasks: %w", err)
	}

	// Start SBI server
	go func() {
		if err := srv.Serve(); err != nil {
			log.WithError(err).Error("SBI server failed")
			cancel()
		}
	}()

	// NRF: self-register (non-fatal if NRF is down).
	nrfURL := viper.GetString("sbi.nrfUrl")
	nrfCli := nrfclient.NewHTTPClient(nrfURL, "SMF", false)
	advertiseAddress := viper.GetString("sbi.advertiseAddress")
	if advertiseAddress == "" {
		advertiseAddress = viper.GetString("sbi.bindAddress")
	}
	smfProfile := &nrfclient.NFProfile{
		NFInstanceID: instanceID,
		NFType:       nrfclient.NFTypeSMF,
		NFStatus:     nrfclient.StatusRegistered,
		Services: []nrfclient.NFService{{
			ServiceName: "nsmf-pdusession",
			Versions:    []string{"v1"},
			Scheme:      "http",
			IPAddr:      advertiseAddress,
			Port:        viper.GetInt("sbi.port"),
		}},
	}
	go nrfclient.NewLifecycleManager(nrfCli, smfProfile, nrfURL, &events.NoopEmitter{}, log).Start(ctx)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.WithField("signal", sig).Info("Shutting down")

	cancel()
	_ = srv.Shutdown(context.Background())
	return nil
}
