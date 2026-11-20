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

// A config written by releases up to 5 holds the detected address in Endpoint
// as well as in PublicIP, because the create path copied it there. That copy
// outranked PublicIP in endpointFor, so Refresh IP never reached the client
// configs; revision 6 clears it. An endpoint the operator typed is kept.
func TestMigrationUnfreezesTheDetectedEndpoint(t *testing.T) {
	cfg := &AppConfig{SchemaVersion: 5, Servers: []api.Server{
		{ID: "auto", Endpoint: "203.0.113.5", PublicIP: "203.0.113.5"},
		{ID: "unknown", Endpoint: "YOUR_SERVER_IP", PublicIP: "YOUR_SERVER_IP"},
		{ID: "named", Endpoint: "vpn.example.com", PublicIP: "203.0.113.5"},
	}}

	if !Migrate(cfg) {
		t.Fatal("a config one revision behind must be reported as changed")
	}
	if cfg.Servers[0].Endpoint != "" {
		t.Errorf("the detected endpoint was kept: %q", cfg.Servers[0].Endpoint)
	}
	if cfg.Servers[1].Endpoint != "" {
		t.Errorf("the placeholder endpoint was kept: %q", cfg.Servers[1].Endpoint)
	}
	if cfg.Servers[2].Endpoint != "vpn.example.com" {
		t.Errorf("the operator's endpoint was cleared: %q", cfg.Servers[2].Endpoint)
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

// A pass that could not give every client an address is not done, and the
// schema is left one revision behind so the next start tries again instead of
// treating those clients as finished for good.
func TestMigrateDoesNotStampAPartialIPv6Pass(t *testing.T) {
	cfg := &AppConfig{
		SchemaVersion: 4,
		Servers: []api.Server{{
			ID:       "s1",
			SubnetV6: "fd42:1::1/128", // no room for a client beside the server
			Clients:  []api.Client{{ID: "c1"}},
		}},
	}

	if !Migrate(cfg) {
		t.Fatal("nothing was migrated")
	}
	if cfg.SchemaVersion != schemaIPv6-1 {
		t.Errorf("schema = %d, want %d so the pass runs again", cfg.SchemaVersion, schemaIPv6-1)
	}
	if addr := cfg.Servers[0].Clients[0].ClientIPV6; addr != "" {
		t.Errorf("client got %q out of a subnet with no room", addr)
	}
}

// A config written before the status field existed stores "" for every client.
// The start-up reconcile treated anything but "active" as parked, so the first
// boot after the upgrade moved a working peer's block out of the live .conf
// while the client list kept showing it as active. Revision 7 fills it in.
func TestMigrationDefaultsAClientStatus(t *testing.T) {
	cfg := &AppConfig{SchemaVersion: 6, Servers: []api.Server{{
		ID: "s1",
		Clients: []api.Client{
			{ID: "old"},
			{ID: "suspended", Status: "suspended"},
		},
	}}}

	if !Migrate(cfg) {
		t.Fatal("a config one revision behind must be reported as changed")
	}
	if got := cfg.Servers[0].Clients[0].Status; got != "active" {
		t.Errorf("an old client kept the empty status %q, want active", got)
	}
	if got := cfg.Servers[0].Clients[1].Status; got != "suspended" {
		t.Errorf("a suspended client became %q", got)
	}
	if cfg.SchemaVersion != SchemaVersion {
		t.Errorf("schema version = %d, want %d", cfg.SchemaVersion, SchemaVersion)
	}
}
