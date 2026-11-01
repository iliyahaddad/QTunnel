package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qtunnel/qtunnel/internal/client"
	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/observability"
	"go.uber.org/zap"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	var (
		configFile = flag.String("c", "", "Configuration file path")
		checkCfg   = flag.Bool("check-config", false, "Validate configuration and exit")
		showVer    = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("qtunnel-client version %s\n", version)
		fmt.Printf("Build time: %s\n", buildTime)
		fmt.Printf("Git commit: %s\n", gitCommit)
		os.Exit(0)
	}

	if *configFile == "" {
		fmt.Fprintln(os.Stderr, "Error: config file required (-c)")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.LoadClientConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	if *checkCfg {
		fmt.Println("Configuration valid")
		os.Exit(0)
	}

	logger, err := observability.NewLogger(&observability.LoggerConfig{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
		Output: cfg.Logging.Output,
		Caller: cfg.Logging.Caller,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger init failed: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	metrics, err := observability.NewMetrics(cfg.Observability.Metrics)
	if err != nil {
		logger.Fatal("Metrics init failed", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("Shutdown signal received")
		cancel()
	}()

	c := client.New(cfg, logger, metrics)
	if err := c.Run(ctx); err != nil {
		logger.Fatal("Client run failed", zap.Error(err))
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := c.Shutdown(shutdownCtx); err != nil {
		logger.Error("Shutdown error", zap.Error(err))
	}

	logger.Info("Client stopped")
}