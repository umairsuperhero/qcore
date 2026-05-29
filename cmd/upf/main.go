package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/upf"
)

func main() {
	var cfgFile string

	rootCmd := &cobra.Command{
		Use:   "upf",
		Short: "QCore User Plane Function",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./upf.yaml)")
	
	viper.SetDefault("upf.pfcpBindAddr", "0.0.0.0:8805")
	viper.SetDefault("upf.gtpuBindAddr", "0.0.0.0:2152")
	viper.SetDefault("upf.tunDeviceName", "qcore-upf")
	
	viper.AutomaticEnv()

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	log := logger.New("debug", "text")
	log.Info("Starting UPF")

	cfg := upf.Config{
		PFCPBindAddr:  viper.GetString("upf.pfcpBindAddr"),
		GTPUBindAddr:  viper.GetString("upf.gtpuBindAddr"),
		TunDeviceName: viper.GetString("upf.tunDeviceName"),
	}

	svc, err := upf.NewService(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to create UPF service: %w", err)
	}
	defer svc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := svc.Start(ctx); err != nil {
			log.WithError(err).Error("UPF service failed")
			cancel()
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.WithField("signal", sig).Info("Shutting down")

	cancel()
	return nil
}
