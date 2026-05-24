package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qcore-project/qcore/pkg/collector"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
	bindAddr  string
	port      int
	logLevel  string
	logFormat string
)

func main() {
	root := &cobra.Command{
		Use:   "qcore-collector",
		Short: "QCore event collector — central hub for structured NF events",
	}
	root.PersistentFlags().StringVar(&bindAddr, "bind", "0.0.0.0", "bind address")
	root.PersistentFlags().IntVar(&port, "port", 9099, "HTTP listen port")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug|info|warn|error)")
	root.PersistentFlags().StringVar(&logFormat, "log-format", "console", "log format (console|json)")

	root.AddCommand(startCmd(), versionCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("QCore Collector\n  Version:  %s\n  Commit:   %s\n  Built:    %s\n",
				version, commit, buildDate)
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the collector server",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := logger.New(logLevel, logFormat)
			log.Info("Starting QCore Collector")

			c := collector.New(log)

			addr := fmt.Sprintf("%s:%d", bindAddr, port)
			srv := &http.Server{
				Addr:         addr,
				Handler:      c.Router(),
				ReadTimeout:  15 * time.Second,
				WriteTimeout: 0, // SSE streams must not time out
				IdleTimeout:  120 * time.Second,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			errCh := make(chan error, 1)
			go func() {
				log.Infof("Collector listening on %s", addr)
				log.Infof("  Ingest:  POST %s/events", addr)
				log.Infof("  Query:   GET  %s/journeys/{id}/events", addr)
				log.Infof("  Stream:  GET  %s/events/stream", addr)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			select {
			case sig := <-sigCh:
				log.Infof("Received %v, shutting down", sig)
			case err := <-errCh:
				return err
			}

			shutCtx, shutCancel := context.WithTimeout(ctx, 10*time.Second)
			defer shutCancel()
			return srv.Shutdown(shutCtx)
		},
	}
}
