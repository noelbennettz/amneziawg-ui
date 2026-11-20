// Package store persists web_config.json - the one file that holds every
// server and client, private keys included - and brings an older copy of it
// up to the current schema.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"amneziawg-web-ui/internal/atomicfile"
	"amneziawg-web-ui/web-ui/api"
)

// AppConfig is the top-level config stored on disk.
type AppConfig struct {
	// SchemaVersion records which one-shot migrations have already been
	// applied to this file, so they don't re-run on every start and undo a
	// choice the operator made afterwards. A file written before this field
	// existed unmarshals to 0.
	SchemaVersion int `json:"schema_version"`

	// Servers owns every client: a client exists exactly once, inside the
	// Clients slice of the server it belongs to. There is no second copy to
	// keep in sync.
	Servers []api.Server `json:"servers"`
}

// Store reads and writes the config file at one path.
type Store struct {
	path string

	// mu orders writes to the file: two concurrent saves cannot reorder
	// into the older one landing last, because each holds mu from the
	// moment it serialises the config until the rename is done.
	mu sync.Mutex
}

// New returns a store over path. Nothing is read until Load.
func New(path string) *Store {
	return &Store{path: path}
}

// Path is the file the store reads and writes.
func (s *Store) Path() string {
	return s.path
}

// Load reads the config. A missing file is a fresh install and comes back as
// an empty config with a nil error. A file that is there but cannot be read or
// parsed is an error, not an empty config: the caller migrates what it loaded
// and writes it straight back, so answering "empty" here is how a corrupt file
// becomes an empty panel with every private key gone.
//
// Slices a file written before they existed leaves nil are made empty, so JSON
// renders them as [] and the rest of the code can append without a nil check.
func (s *Store) Load() (*AppConfig, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &AppConfig{Servers: []api.Server{}}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", s.path, err)
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.path, err)
	}
	if cfg.Servers == nil {
		cfg.Servers = []api.Server{}
	}
	for i := range cfg.Servers {
		if cfg.Servers[i].Clients == nil {
			cfg.Servers[i].Clients = []api.Client{}
		}
		if cfg.Servers[i].UnboundNATIPs == nil {
			cfg.Servers[i].UnboundNATIPs = []string{}
		}
		if cfg.Servers[i].UnboundNATIPsV6 == nil {
			cfg.Servers[i].UnboundNATIPsV6 = []string{}
		}
	}
	return &cfg, nil
}

// Save writes cfg to disk atomically. The caller must keep cfg from changing
// for the duration - the manager holds its read lock across the call - since
// the snapshot is taken here, under mu, so that concurrent saves land in the
// order they serialised.
func (s *Store) Save(cfg *AppConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := atomicfile.Write(s.path, data, 0o600); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	return nil
}
