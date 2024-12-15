package dotenv

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hyp3rd/base/internal/secrets"
	"github.com/hyp3rd/base/internal/secrets/encryption"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

// EncryptedProvider is a provider that encrypts and decrypts secrets using a cryptographer.
type EncryptedProvider struct {
	*Provider
	crypto *encryption.Cryptographer
}

// NewEncrypted creates a new EncryptedProvider instance with the given configuration and password.
// The EncryptedProvider wraps a base Provider and uses the provided password to encrypt and decrypt secrets.
// If an error occurs during initialization, it is returned.
func NewEncrypted(config secrets.Config, password string) (*EncryptedProvider, error) {
	baseProvider, err := New(config)
	if err != nil {
		return nil, err
	}

	crypto, err := encryption.New(password)
	if err != nil {
		return nil, ewrap.Wrapf(err, "initializing cryptographer")
	}

	return &EncryptedProvider{
		Provider: baseProvider,
		crypto:   crypto,
	}, nil
}

// GetSecret retrieves a secret from the encrypted provider. If the secret is encrypted, it will decrypt the value before returning it.
// If the secret is not encrypted, it will simply return the unencrypted value.
// If an error occurs during the retrieval or decryption of the secret, the error is returned.
func (p *EncryptedProvider) GetSecret(ctx context.Context, key string) (string, error) {
	encryptedValue, err := p.Provider.GetSecret(ctx, key)
	if err != nil {
		return "", err
	}

	// Check if the value is actually encrypted
	if !strings.HasPrefix(encryptedValue, "ENC[") {
		return encryptedValue, nil // Return unencrypted value
	}

	// Extract the encrypted portion
	encryptedValue = strings.TrimPrefix(encryptedValue, "ENC[")
	encryptedValue = strings.TrimSuffix(encryptedValue, "]")

	// Decrypt the value
	decryptedValue, err := p.crypto.Decrypt(encryptedValue)
	if err != nil {
		return "", ewrap.Wrapf(err, "decrypting secret").
			WithMetadata("key", key)
	}

	return decryptedValue, nil
}

// SetSecret encrypts the given value and stores it in the underlying provider, prefixing the encrypted value with "ENC[" and suffixing it with "]".
// If an error occurs during the encryption of the value, it is returned.
func (p *EncryptedProvider) SetSecret(ctx context.Context, key, value string) error {
	// Encrypt the value
	encryptedValue, err := p.crypto.Encrypt(value)
	if err != nil {
		return ewrap.Wrapf(err, "encrypting secret").
			WithMetadata("key", key)
	}

	// Store with encryption marker
	return p.Provider.SetSecret(ctx, key, fmt.Sprintf("ENC[%s]", encryptedValue))
}

// EncryptFile encrypts the contents of the input file and writes the encrypted contents to the output file.
// The function reads each line from the input file, and if the line is not a comment or empty, it encrypts the value
// and writes the encrypted line to the output file. If the value is already encrypted, it is written to the output
// file without further encryption.
func (p *EncryptedProvider) EncryptFile(inputPath, outputPath string) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return ewrap.Wrapf(err, "opening input file")
	}
	defer input.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return ewrap.Wrapf(err, "creating output file")
	}
	defer output.Close()

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			// Preserve comments and empty lines
			fmt.Fprintln(output, line)

			continue
		}

		// Parse the line
		//nolint:mnd
		parts := strings.SplitN(line, "=", 2)
		//nolint:mnd
		if len(parts) != 2 {
			continue // Skip invalid lines
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Don't encrypt already encrypted values
		if strings.HasPrefix(value, "ENC[") {
			fmt.Fprintln(output, line)

			continue
		}

		// Encrypt the value
		encryptedValue, err := p.crypto.Encrypt(value)
		if err != nil {
			return ewrap.Wrapf(err, "encrypting value").
				WithMetadata("key", key)
		}

		// Write the encrypted line
		fmt.Fprintf(output, "%s=ENC[%s]\n", key, encryptedValue)
	}

	err = scanner.Err()
	if err != nil {
		return ewrap.Wrapf(err, "error reading input file while encrypting secrets file")
	}

	return nil
}
