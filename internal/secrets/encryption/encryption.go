package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/hyp3rd/ewrap/pkg/ewrap"
	"golang.org/x/crypto/scrypt"
)

const (
	// KeyLength is the length of the key used for encryption.
	KeyLength = 32
	// ResourceCost is the cost of the scrypt key derivation function.
	ResourceCost = 1 << 15
	// BlockSize is the block size of the cipher.
	BlockSize = 8
)

// Metadata holds the parameters needed for decryption.
type Metadata struct {
	Version    int                 `json:"v"` // Version of the encryption format
	Salt       []byte              `json:"s"` // Salt used for key derivation
	Params     KeyDerivationParams `json:"p"` // Key derivation parameters
	Nonce      []byte              `json:"n"` // Nonce used for encryption
	Ciphertext []byte              `json:"c"` // The encrypted data
}

// KeyDerivationParams defines the parameters for key derivation using scrypt.
type KeyDerivationParams struct {
	// Salt   []byte // Salt for key derivation
	N      int `json:"n"`  // CPU/memory cost parameter (must be power of 2)
	R      int `json:"r"`  // Block size parameter
	P      int `json:"p"`  // Parallelization parameter
	KeyLen int `json:"kl"` // Length of the derived key
}

// DefaultParams returns secure default parameters for key derivation.
func DefaultParams() KeyDerivationParams {
	return KeyDerivationParams{
		// Salt:   make([]byte, KeyLength), // 32-byte salt
		N:      ResourceCost, // CPU/memory cost (32768)
		R:      BlockSize,    // Block size
		P:      1,            // Parallelization
		KeyLen: KeyLength,    // 256-bit key
	}
}

// Cryptographer handles encryption and decryption of secrets.
type Cryptographer struct {
	mu       sync.RWMutex
	params   KeyDerivationParams
	password []byte
}

// New creates a new Cryptographer instance.
func New(password string) (*Cryptographer, error) {
	cryptographer := &Cryptographer{
		params: DefaultParams(),
	}

	cryptographer.password = []byte(password)

	// Generate a random salt if not provided
	// if _, err := io.ReadFull(rand.Reader, cryptographer.params.Salt); err != nil {
	// 	return nil, ewrap.Wrapf(err, "generating random salt")
	// }

	// Initialize the cryptographer with the password
	// if err := cryptographer.Initialize(password); err != nil {
	// 	return nil, err
	// }

	return cryptographer, nil
}

// Initialize sets up the cryptographer with a password.
// func (c *Cryptographer) Initialize(password string) error {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()

// 	// Derive the encryption key from the password
// 	// key, err := c.deriveKey(password)
// 	// if err != nil {
// 	// 	return ewrap.Wrapf(err, "deriving encryption key")
// 	// }

// 	// Create cipher block
// 	block, err := aes.NewCipher(key)
// 	if err != nil {
// 		return ewrap.Wrapf(err, "creating cipher block")
// 	}

// 	// Create GCM mode
// 	gcm, err := cipher.NewGCM(block)
// 	if err != nil {
// 		return ewrap.Wrapf(err, "creating GCM mode")
// 	}

// 	c.gcm = gcm
// 	c.masterKey = key
// 	c.initialized = true

// 	return nil
// }

// Encrypt encrypts a plaintext string and returns a formatted encrypted string.
func (c *Cryptographer) Encrypt(plaintext string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Generate a random salt
	salt := make([]byte, KeyLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", ewrap.Wrapf(err, "generating salt")
	}

	// Derive the key
	key, err := scrypt.Key(c.password, salt, c.params.N, c.params.R, c.params.P, c.params.KeyLen)
	if err != nil {
		return "", ewrap.Wrapf(err, "deriving key")
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ewrap.Wrapf(err, "creating cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ewrap.Wrapf(err, "creating GCM")
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", ewrap.Wrapf(err, "generating nonce")
	}

	// Encrypt the data
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Create metadata
	metadata := Metadata{
		Version:    1,
		Salt:       salt,
		Params:     c.params,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}

	// Serialize metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", ewrap.Wrapf(err, "marshaling metadata")
	}

	// Encode everything in base64
	encoded := base64.StdEncoding.EncodeToString(metadataJSON)

	return fmt.Sprintf("ENC[%s]", encoded), nil
}

// Decrypt decrypts a formatted encrypted string using the provided key.
func (c *Cryptographer) Decrypt(encryptedData string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Remove the ENC[] wrapper
	if !strings.HasPrefix(encryptedData, "ENC[") || !strings.HasSuffix(encryptedData, "]") {
		return "", ewrap.New("invalid encryption format")
	}

	encoded := encryptedData[4 : len(encryptedData)-1]

	// Decode the base64 data
	metadataJSON, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ewrap.Wrapf(err, "decoding base64")
	}

	// Unmarshal metadata
	var metadata Metadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return "", ewrap.Wrapf(err, "unmarshaling metadata")
	}

	// Derive the key using the stored parameters
	key, err := scrypt.Key(
		c.password,
		metadata.Salt,
		metadata.Params.N,
		metadata.Params.R,
		metadata.Params.P,
		metadata.Params.KeyLen,
	)
	if err != nil {
		return "", ewrap.Wrapf(err, "deriving key")
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ewrap.Wrapf(err, "creating cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ewrap.Wrapf(err, "creating GCM")
	}

	// Decrypt the data
	plaintext, err := gcm.Open(nil, metadata.Nonce, metadata.Ciphertext, nil)
	if err != nil {
		return "", ewrap.Wrapf(err, "decrypting data")
	}

	return string(plaintext), nil
}

// func (c *Cryptographer) deriveKey(password string) ([]byte, error) {
// 	bytes, err := scrypt.Key(
// 		[]byte(password),
// 		c.params.Salt,
// 		c.params.N,
// 		c.params.R,
// 		c.params.P,
// 		c.params.KeyLen,
// 	)
// 	if err != nil {
// 		return nil, ewrap.Wrapf(err, "error deriving key")
// 	}

// 	return bytes, nil
// }

// // RotateKey safely rotates the encryption key.
// func (c *Cryptographer) RotateKey(newPassword string) error {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()

// 	// Create a temporary cryptographer with the new key
// 	newCrypto, err := New(newPassword)
// 	if err != nil {
// 		return ewrap.Wrapf(err, "creating new cryptographer")
// 	}

// 	// Update the current cryptographer with the new key
// 	c.gcm = newCrypto.gcm
// 	c.params = newCrypto.params
// 	c.masterKey = newCrypto.masterKey

// 	return nil
// }
