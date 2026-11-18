package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amneziawg-web-ui/internal/wgconf"
	"amneziawg-web-ui/web-ui/api"
)

// addClient puts a client in the test manager's one server.
func addClient(m *Manager, c api.Client) {
	m.cfg.Servers[0].Clients = append(m.cfg.Servers[0].Clients, c)
}

// The .conf is derived from the model, so a missing one is rebuilt rather than
// left missing: the interface section comes from the server, the peers from the
// clients, and the server can be started again.
func TestReconcileRebuildsAMissingServerConf(t *testing.T) {
	m, _ := newTestManager(t)
	srv := m.cfg.Servers[0]
	srv.ServerPrivateKey = "SERVPRIV"
	srv.ServerPublicKey = "SERVPUB"
	srv.SubnetV6 = "fd42:ab12::/64"
	srv.ServerIPV6 = "fd42:ab12::1"
	m.cfg.Servers[0] = srv

	alice := api.Client{
		ID: "aaa", Name: "alice", Status: "active", ServerID: srv.ID,
		ClientPublicKey: "APUB", PresharedKey: "APSK", ClientIP: "10.0.1.2",
	}
	addClient(m, alice)

	if err := os.Remove(srv.ConfigPath); err != nil {
		t.Fatal(err)
	}

	m.reconcilePeers()

	data, err := os.ReadFile(srv.ConfigPath)
	if err != nil {
		t.Fatalf("the config was not rebuilt: %v", err)
	}
	conf := string(data)
	for _, want := range []string{
		"PrivateKey = SERVPRIV",
		"Address = 10.0.1.1/24, fd42:ab12::1/64",
		"ListenPort = 54844",
		"MTU = 1280",
		"PublicKey = APUB",
		"AllowedIPs = 10.0.1.2/32",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("rebuilt config is missing %q:\n%s", want, conf)
		}
	}
	// Obfuscation is part of the interface, not of the peers: a server that
	// runs AmneziaWG must not come back as plain WireGuard.
	if !strings.Contains(conf, "S1 = 50") {
		t.Errorf("rebuilt config lost the obfuscation parameters:\n%s", conf)
	}
}

// Without the key there is nothing to rebuild from, and saying so is the whole
// point: a silently missing server is one the operator debugs by hand.
func TestReconcileRefusesToRebuildWithoutAPrivateKey(t *testing.T) {
	m, _ := newTestManager(t)
	srv := m.cfg.Servers[0]
	if err := os.Remove(srv.ConfigPath); err != nil {
		t.Fatal(err)
	}

	err := m.ensureServerConf(srv)
	if err == nil || !strings.Contains(err.Error(), "no private key") {
		t.Fatalf("ensureServerConf = %v, want an error about the missing key", err)
	}
	if _, statErr := os.Stat(srv.ConfigPath); !os.IsNotExist(statErr) {
		t.Error("a config was written with no key to put in it")
	}
}

// A file that exists is left alone: rebuilding on every start would throw away
// hand-written peer blocks, which are the one part of the file that is not the
// model's.
func TestReconcileLeavesAnExistingConfAlone(t *testing.T) {
	m, _ := newTestManager(t)
	srv := m.cfg.Servers[0]
	handWritten := "[Interface]\nPrivateKey = HAND\n\n[Peer]\nPublicKey = WROTE\n"
	if err := os.WriteFile(srv.ConfigPath, []byte(handWritten), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := m.ensureServerConf(srv); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(srv.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != handWritten {
		t.Errorf("an existing config was rewritten:\n%s", data)
	}
}

// Deleting a server deletes its suspended clients' parked blocks with it. Left
// behind, they are keys on disk that nothing owns, and activation reads them
// back by client ID.
func TestDeleteServerRemovesItsParkedPeers(t *testing.T) {
	m, _ := newTestManager(t)
	srv := m.cfg.Servers[0]
	alice := api.Client{
		ID: "aaa", Name: "alice", Status: "active", ServerID: srv.ID,
		ClientPublicKey: "APUB", PresharedKey: "APSK", ClientIP: "10.0.1.2",
	}
	addClient(m, alice)
	if err := wgconf.AppendPeer(srv.ConfigPath, &alice, wgconf.PeerAllowedIPs(&alice)); err != nil {
		t.Fatal(err)
	}
	if err := wgconf.ParkPeer(srv.ConfigPath, &alice); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(wgconf.SuspendedDir(srv.ConfigPath), "aaa.conf")
	if _, err := os.Stat(parked); err != nil {
		t.Fatalf("the test did not park anything: %v", err)
	}

	if err := m.DeleteServer(srv.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(parked); !os.IsNotExist(err) {
		t.Errorf("the parked block outlived its server (err %v)", err)
	}
	if _, err := os.Stat(wgconf.SuspendedDir(srv.ConfigPath)); !os.IsNotExist(err) {
		t.Errorf("the empty suspended directory was left behind (err %v)", err)
	}
	if _, err := os.Stat(srv.ConfigPath); !os.IsNotExist(err) {
		t.Errorf("the server config outlived its server (err %v)", err)
	}
}

// Another server's parked client lives in the same directory and must survive.
func TestDeleteServerLeavesOtherServersParkedPeersAlone(t *testing.T) {
	m, _ := newTestManager(t)
	srv := m.cfg.Servers[0]
	other := filepath.Join(wgconf.SuspendedDir(srv.ConfigPath), "bbb.conf")
	if err := os.MkdirAll(wgconf.SuspendedDir(srv.ConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("PublicKey = BPUB\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := m.DeleteServer(srv.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(other); err != nil {
		t.Errorf("a parked peer of another server was deleted: %v", err)
	}
}

// A start is not failed by a firewall it could not set up: the tunnel is up,
// and the panel's job is then to say so loudly rather than to tear it down.
// The warning is on stdout; what this pins down is that the error does not
// travel up as a failed start.
func TestStartSucceedsWhenTheFirewallScriptIsMissing(t *testing.T) {
	m, run := newTestManager(t)
	srv := m.cfg.Servers[0]
	run.Stub("/usr/bin/awg-quick up", "")

	if err := m.StartServer(srv.ID); err != nil {
		t.Fatalf("StartServer = %v, want success with only a warning", err)
	}
	if srv, ok := m.Server(srv.ID); !ok || srv.Status != "running" {
		t.Errorf("status after start = %q (found %v), want running", srv.Status, ok)
	}
}

func TestDeleteServerForgetsTheQuietMark(t *testing.T) {
	m, _ := newTestManager(t)
	srv := m.cfg.Servers[0]
	m.markQuiet(srv.Interface)

	if err := m.DeleteServer(srv.ID); err != nil {
		t.Fatal(err)
	}

	if m.isQuiet(srv.Interface) {
		t.Error("the deleted server's interface is still marked quiet")
	}
	m.quietMu.Lock()
	defer m.quietMu.Unlock()
	if _, ok := m.quiet[srv.Interface]; ok {
		t.Error("the quiet mark was left in the map")
	}
}
