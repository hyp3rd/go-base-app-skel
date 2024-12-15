package secrets

import (
	"context"
	"sync"

	"github.com/hyp3rd/base/internal/constants"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

// Manager is the main struct responsible for managing secrets in the application.
// It holds a reference to the secrets store and the provider that retrieves the secrets.
// The Manager is thread-safe and uses a read-write mutex to protect the secrets store.
type Manager struct {
	Provider Provider
	store    *Store
	mu       sync.RWMutex
}

// NewManager creates a new Manager instance with the provided Provider.
// The Manager is responsible for managing secrets in the application.
func NewManager(provider Provider) *Manager {
	return &Manager{
		Provider: provider,
		store:    &Store{},
	}
}

// Load loads the secrets from the provider and stores them in the Manager's secrets store.
// It first loads the database credentials, then the API keys, and finally validates the loaded secrets.
// If any error occurs during the loading process, the function will return the error.
func (m *Manager) Load(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load database credentials
	if err := m.loadSecret(ctx, constants.DBUsername.String(), &m.store.DBCredentials.Username); err != nil {
		return err
	}

	if err := m.loadSecret(ctx, constants.DBPassword.String(), &m.store.DBCredentials.Password); err != nil {
		return err
	}

	// Load other secrets
	// ...

	return m.validate()
}

// GetStore returns a copy of the Manager's secrets store to prevent external modifications.
// The returned store is a deep copy, so changes to the copy will not affect the original store.
// The method acquires a read lock on the Manager's mutex to ensure thread-safety.
func (m *Manager) GetStore() *Store {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modifications
	storeCopy := *m.store

	return &storeCopy
}

// SetStore sets the Manager's secrets store to the provided value.
// The method acquires a write lock on the Manager's mutex to ensure thread-safety.
// It returns the updated store.
func (m *Manager) SetStore(secrets *Store) *Store {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set the store to the provided value
	m.store = secrets

	return m.store
}

func (m *Manager) loadSecret(ctx context.Context, key string, target *string) error {
	value, err := m.Provider.GetSecret(ctx, key)
	if err != nil {
		return ewrap.Wrapf(err, "loading secret").
			WithMetadata("key", key)
	}

	*target = value

	return nil
}

func (m *Manager) validate() error {
	if m.store.DBCredentials.Username == "" || m.store.DBCredentials.Password == "" {
		return ewrap.New("database credentials are required")
	}

	// Validate other secrets here

	return nil
}
