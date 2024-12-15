package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hyp3rd/base/internal/config"
	"github.com/hyp3rd/base/internal/constants"
	"github.com/hyp3rd/base/internal/logger"
	"github.com/hyp3rd/base/internal/logger/adapter"
	"github.com/hyp3rd/base/internal/logger/output"
	"github.com/hyp3rd/base/internal/repository/pg"
	"github.com/hyp3rd/base/internal/secrets"
	"github.com/hyp3rd/base/internal/secrets/providers/dotenv"
)

const (
	maxLogSize = 10 * 1024 * 1024 // 10 MB
	logsDir    = "logs/pg/monitor"
	logsFile   = "pg-monitor.log"

	configFileName = "config"

	monitorInterval = 10 * time.Second
)

func main() {
	ctx := context.Background()

	cfg := initConfig(ctx)
	log, multiWriter := initLogger(ctx, cfg.Environment)
	// Ensure proper cleanup with detailed error handling
	defer func() {
		if err := multiWriter.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Logger sync failed: %+v\n", err)
		}

		if err := multiWriter.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Writer cleanup failed: %+v\n", err)
		}
	}()

	log.Info("Database monitor starting")

	dbManager := initDBmanager(ctx, cfg, log)

	// Create monitor with 1 second slow query threshold
	monitor := dbManager.NewMonitor(time.Second)

	// Start monitoring
	monitor.Start(ctx)
	defer monitor.Stop()

	// Create a ticker for periodic checks
	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Main process loop
	for {
		select {
		case <-ticker.C:
			status := monitor.GetHealthStatus()
			if !status.Connected {
				log.Error("Database connection lost!")
			} else {
				log.Info("Database connection is healthy")
			}

			if status.PoolStats != nil {
				if status.PoolStats.SlowQueries > 0 {
					log.Warn("Detected slow queries")
				}
			}

		case sig := <-sigChan:
			log.Infof("Received signal: %v, shutting down...", sig)

			return
		case <-ctx.Done():
			log.Info("Context cancelled, shutting down...")

			return
		}
	}
}

func initConfig(ctx context.Context) *config.Config {
	// Initialize the encrypted provider
	secretsProviderCfg := secrets.Config{
		Source:  secrets.EnvFile,
		Prefix:  constants.EnvPrefix.String(),
		EnvPath: ".env.encrypted",
	}

	encryptionPassword, ok := os.LookupEnv("SECRETS_ENCRYPTION_PASSWORD")
	if !ok {
		fmt.Fprintf(os.Stderr, "SECRETS_ENCRYPTION_PASSWORD environment variable not set\n")
		os.Exit(1)
	}

	secretsProvider, err := dotenv.NewEncrypted(secretsProviderCfg, encryptionPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Secrets provider: %+v\n", err)
		os.Exit(1)
	}

	// Configure options for config initialization
	opts := config.Options{
		ConfigName:      configFileName,
		SecretsProvider: secretsProvider,
		Timeout:         constants.DefaultTimeout,
	}

	cfg, err := config.NewConfig(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize config: %v\n", err)
		os.Exit(1)
	}

	return cfg
}

func initLogger(_ context.Context, environment string) (logger.Logger, *output.MultiWriter) {
	//nolint:mnd
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
		os.Exit(1)
	}

	// Create file writer with proper error handling
	fileWriter, err := output.NewFileWriter(output.FileConfig{
		Path:     logsDir + "/" + logsFile,
		MaxSize:  maxLogSize,
		Compress: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create file writer: %v\n", err)
		os.Exit(1)
	}

	// Create console writer
	consoleWriter := output.NewConsoleWriter(os.Stdout, output.ColorModeAuto)

	// Create multi-writer with error handling
	multiWriter, err := output.NewMultiWriter(consoleWriter, fileWriter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create multi-writer: %v\n", err)
		fileWriter.Close() // Clean up the file writer
		os.Exit(1)
	}

	// Initialize the logger
	loggerCfg := logger.DefaultConfig()
	loggerCfg.Output = multiWriter
	loggerCfg.EnableJSON = true
	loggerCfg.TimeFormat = time.RFC3339
	loggerCfg.EnableCaller = true
	loggerCfg.Level = logger.DebugLevel
	loggerCfg.AdditionalFields = []logger.Field{
		{Key: "service", Value: "database-monitor"},
		{Key: "environment", Value: environment},
	}

	// Create the logger
	log, err := adapter.NewAdapter(loggerCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %+v\n", err)
		os.Exit(1)
	}

	return log, multiWriter
}

func initDBmanager(ctx context.Context, cfg *config.Config, log logger.Logger) *pg.Manager {
	// Initialize the database manager
	dbManager := pg.New(&cfg.DB, log)

	err := dbManager.Connect(ctx)
	if err != nil {
		log.Error("Failed to connect to database")
		panic(err)
	}

	if dbManager.IsConnected(ctx) {
		log.Info("Database connection successfully established")
	} else {
		log.Error("Database connection wasn't established")
	}

	return dbManager
}
