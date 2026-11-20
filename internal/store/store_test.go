package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"amneziawg-web-ui/web-ui/api"
)

func TestLoadOfAMissingFileIsAnEmptyConfig(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "web_config.json"))
	cfg, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Servers == nil || len(cfg.Servers) != 0 {
		t.Errorf("Load = %+v, want an empty server list", cfg)
	}
}

// A file that exists but cannot be read back is an error, not an empty
// config: the manager saves what it loaded, so answering "empty" would let a
// corrupt file overwrite itself with a panel that has no servers at all.
func TestLoadOfACorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_config.json")
	if err := os.WriteFile(path, []byte(`{"servers":[{"id":"s1"`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := New(path).Load(); err == nil {
		t.Fatal("Load of a truncated config returned no error")
	}
}

// A file written before Clients and UnboundNATIPs existed leaves them nil;
// the loaded config must not, or JSON renders null and appends need guards.
func TestLoadFillsInMissingSlices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_config.json")
	if err := os.WriteFile(path, []byte(`{"servers":[{"id":"s1"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := New(path).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers[0].Clients == nil || cfg.Servers[0].UnboundNATIPs == nil {
		t.Errorf("Load = %+v", cfg)
	}
}

func TestSaveRoundTrips(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "web_config.json"))
	cfg := &AppConfig{SchemaVersion: SchemaVersion, Servers: []api.Server{{
		ID: "s1", Clients: []api.Client{{ID: "c1", ServerID: "s1"}},
	}}}
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SchemaVersion != SchemaVersion || len(got.Servers) != 1 || len(got.Servers[0].Clients) != 1 {
		t.Errorf("Load after Save = %+v", got)
	}
}

func TestConcurrentSavesLeaveAValidConfig(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "web_config.json"))
	cfg := &AppConfig{Servers: []api.Server{{ID: "s1", Clients: []api.Client{{ID: "c1"}}}}}

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if err := s.Save(cfg); err != nil {
				t.Errorf("Save: %v", err)
			}
		})
	}
	wg.Wait()

	data, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	var got AppConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("config is not valid JSON after concurrent saves: %v\n%s", err, data)
	}
	if len(got.Servers) != 1 || len(got.Servers[0].Clients) != 1 {
		t.Errorf("config lost data: %+v", got)
	}
}
