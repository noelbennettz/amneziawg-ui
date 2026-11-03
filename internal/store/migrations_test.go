package store

import (
	"testing"

	"amneziawg-web-ui/web-ui/api"
)

// The stamp is the part every migration shares: a config that predates the
// field is brought up to the current revision and reported as changed, and
// one that is already current is left alone.
func TestMigrateStampsAnUnversionedConfig(t *testing.T) {
	cfg := &AppConfig{Servers: []api.Server{{ID: "s1"}}}
	if !Migrate(cfg) {
		t.Error("an unversioned config must be reported as changed")
	}
	if got := cfg.SchemaVersion; got != SchemaVersion {
		t.Errorf("schema version = %d, want %d", got, SchemaVersion)
	}
}

func TestMigrateLeavesACurrentConfigAlone(t *testing.T) {
	cfg := &AppConfig{SchemaVersion: SchemaVersion}
	if Migrate(cfg) {
		t.Error("a config that needs nothing must not be reported as changed")
	}
}

// Configs written before 3.1.5 can hold a client ServerID that decayed into a
// slice of an unrelated request path, because fiber recycles the buffer a
// route parameter points into. The server a client is stored under is what
// binds them, so the migration rewrites the copy from it.
func TestMigrationRepairsClientServerIDs(t *testing.T) {
	cfg := &AppConfig{SchemaVersion: 3, Servers: []api.Server{{
		ID: "s1",
		Clients: []api.Client{
			{ID: "c1", ServerID: "tatus5"},
			{ID: "c2", ServerID: "s1"},
		},
	}, {
		ID:      "s2",
		Clients: []api.Client{{ID: "c3", ServerID: "i-sett"}},
	}}}

	Migrate(cfg)

	for _, srv := range cfg.Servers {
		for _, c := range srv.Clients {
			if c.ServerID != srv.ID {
				t.Errorf("client %s: ServerID = %q, want %q", c.ID, c.ServerID, srv.ID)
			}
		}
	}
	if got := cfg.SchemaVersion; got != SchemaVersion {
		t.Errorf("schema version = %d, want %d", got, SchemaVersion)
	}
}

// Configs written before IPv6 support have no IPv6 addressing at all. The
// migration gives every server a ULA /64 - named after its ID, and never one
// another server already holds - the server the first address in it, and every
// client one after that. An address a client already carries is left alone.
func TestMigrationAddsIPv6Addressing(t *testing.T) {
	cfg := &AppConfig{SchemaVersion: 4, Servers: []api.Server{{
		ID: "ab12cd", Subnet: "10.0.1.0/24", ServerIP: "10.0.1.1",
		Clients: []api.Client{
			{ID: "c1", ClientIP: "10.0.1.2"},
			{ID: "c2", ClientIP: "10.0.1.3", ClientIPV6: "fd42:ab12::9"},
			{ID: "c3", ClientIP: "10.0.1.4"},
		},
	}, {
		// Same seed group as the server above: the migration has to move on to
		// the next one instead of handing out a second fd42:ab12::/64.
		ID: "ab12ef", Subnet: "10.0.2.0/24", ServerIP: "10.0.2.1",
	}}}

	Migrate(cfg)

	first := cfg.Servers[0]
	if first.SubnetV6 != "fd42:ab12::/64" || first.ServerIPV6 != "fd42:ab12::1" {
		t.Errorf("server 1 = %q / %q, want fd42:ab12::/64 / fd42:ab12::1", first.SubnetV6, first.ServerIPV6)
	}
	want := map[string]string{"c1": "fd42:ab12::2", "c2": "fd42:ab12::9", "c3": "fd42:ab12::3"}
	for _, c := range first.Clients {
		if c.ClientIPV6 != want[c.ID] {
			t.Errorf("client %s: ClientIPV6 = %q, want %q", c.ID, c.ClientIPV6, want[c.ID])
		}
	}

	second := cfg.Servers[1]
	if second.SubnetV6 != "fd42:ab13::/64" || second.ServerIPV6 != "fd42:ab13::1" {
		t.Errorf("server 2 = %q / %q, want fd42:ab13::/64 / fd42:ab13::1", second.SubnetV6, second.ServerIPV6)
	}
}
