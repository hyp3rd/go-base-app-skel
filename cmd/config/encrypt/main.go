package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/hyp3rd/base/internal/constants"
	"github.com/hyp3rd/base/internal/secrets"
	"github.com/hyp3rd/base/internal/secrets/providers/dotenv"
)

const (
	sourceEnvFile    = ".env"
	encryptedEnvFile = ".env.encrypted"
)

func main() {
	encryptionPassword, ok := os.LookupEnv("SECRETS_ENCRYPTION_PASSWORD")
	if !ok {
		fmt.Fprintf(os.Stderr, "SECRETS_ENCRYPTION_PASSWORD environment variable not set\n")
		os.Exit(1)
	}

	// Initialize the encrypted provider
	secretsProviderCfg := secrets.Config{
		Source:  secrets.EnvFile,
		Prefix:  constants.EnvPrefix.String(),
		EnvPath: encryptedEnvFile,
	}

	provider, err := dotenv.NewEncrypted(secretsProviderCfg, encryptionPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initiate the configuration encryption provider: %v\n", err)
		os.Exit(1)
	}

	// Encrypt the existing .env file
	err = provider.EncryptFile(sourceEnvFile, encryptedEnvFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encrypt the .env provided: %v\n", err)
		os.Exit(1)
	}

	slog.Info("Encryption complete")
}
