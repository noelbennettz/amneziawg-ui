package wgconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amneziawg-web-ui/web-ui/api"
)

// writeConf lays out a server .conf with the interface header and whatever
// blocks the test wants in it, and returns the path.
func writePeerConf(t *testing.T, blocks ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "wg-test.conf")
	conf := "[Interface]\nPrivateKey = key\nAddress = 10.0.0.1/24\nListenPort = 54844\n"
	for _, block := range blocks {
		conf += block
	}
	if err := os.WriteFile(path, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testClient(id, name, status string) api.Client {
	return api.Client{
		ID:              id,
		Name:            name,
		Status:          status,
		ClientPublicKey: "pub-" + id,
		PresharedKey:    "psk-" + id,
		ClientIP:        "10.0.0.2",
		ClientIPV6:      "fd42:1::2",
		AllowedIPs:      "0.0.0.0/0, ::/0",
	}
}

// A config that already agrees with the model is not touched at all: the pass
// runs on every start, and rewriting the file for nothing would pointlessly
// disturb every tunnel that is watching it.
func TestReconcileLeavesAGreeingConfigAlone(t *testing.T) {
	client := testClient("ab12cd", "alice", "active")
	path := writePeerConf(t, PeerBlock(&client, PeerAllowedIPs(&client)))

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Reconcile(path, []api.Client{client})
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed() || result.Unattributed != 0 {
		t.Errorf("reconcile changed a config that agreed: %+v", result)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the file was rewritten")
	}
}

// The crash this exists for: the client is in the model and its block never
// made it into the file, or was lost with it.
func TestReconcileRestoresAMissingPeer(t *testing.T) {
	client := testClient("ab12cd", "alice", "active")
	path := writePeerConf(t)

	result, err := Reconcile(path, []api.Client{client})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 1 || result.Added[0] != "alice" {
		t.Errorf("result = %+v, want alice restored", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Client: alice [id:ab12cd]", "[Peer]", "PublicKey = pub-ab12cd", "PresharedKey = psk-ab12cd"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the restored block is missing %q:\n%s", want, data)
		}
	}

	// And the second pass has nothing left to do.
	again, err := Reconcile(path, []api.Client{client})
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed() {
		t.Errorf("a second pass changed something: %+v", again)
	}
}

// The model says suspended, the file still has the peer: the block belongs in
// the parked directory under the client's own name, which is where activation
// reads it from.
func TestReconcileParksAPeerTheModelSuspended(t *testing.T) {
	client := testClient("ab12cd", "alice", "suspended")
	path := writePeerConf(t, PeerBlock(&client, PeerAllowedIPs(&client)))

	result, err := Reconcile(path, []api.Client{client})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Parked) != 1 {
		t.Fatalf("result = %+v, want one parked peer", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "pub-ab12cd") {
		t.Errorf("the suspended peer is still in the live config:\n%s", data)
	}

	parked, err := os.ReadFile(filepath.Join(SuspendedDir(path), "ab12cd.conf"))
	if err != nil {
		t.Fatalf("the block was not parked: %v", err)
	}
	if !strings.Contains(string(parked), "pub-ab12cd") {
		t.Errorf("the parked block is not the client's:\n%s", parked)
	}
}

// A peer the model does not know is the dangerous one: it still connects. It
// is parked, not deleted, under a name activation never looks for.
func TestReconcileParksAnUnknownPeer(t *testing.T) {
	orphan := testClient("de9999", "ghost", "active")
	path := writePeerConf(t, PeerBlock(&orphan, PeerAllowedIPs(&orphan)))

	result, err := Reconcile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Orphans) != 1 || !strings.Contains(result.Orphans[0], "de9999") {
		t.Fatalf("result = %+v, want the ghost peer reported", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "pub-de9999") {
		t.Errorf("an unknown peer is still live:\n%s", data)
	}

	parked, err := os.ReadFile(filepath.Join(SuspendedDir(path), "orphan-de9999.conf"))
	if err != nil {
		t.Fatalf("the orphan was not parked: %v", err)
	}
	if !strings.Contains(string(parked), "pub-de9999") {
		t.Errorf("the parked orphan is not the block that was removed:\n%s", parked)
	}
}

// A block with no client marker is somebody else's: it stays where it is and
// is counted, because nothing here can say whose it is.
func TestReconcileLeavesHandWrittenPeersAlone(t *testing.T) {
	path := writePeerConf(t, "\n[Peer]\nPublicKey = handwritten\nAllowedIPs = 10.0.0.9/32\n")

	result, err := Reconcile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Unattributed != 1 || result.Changed() {
		t.Errorf("result = %+v, want one unattributed block and no change", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "PublicKey = handwritten") {
		t.Errorf("the hand-written block was removed:\n%s", data)
	}
}

// Deleting a client has to take its parked block with it; forgetting a block
// that was never parked is not an error.
func TestForgetPeer(t *testing.T) {
	path := writePeerConf(t)
	if err := ParkLines(path, "ab12cd", []string{"# Client: alice [id:ab12cd]", "[Peer]"}); err != nil {
		t.Fatal(err)
	}

	if err := ForgetPeer(path, "ab12cd"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(SuspendedDir(path), "ab12cd.conf")); !os.IsNotExist(err) {
		t.Error("the parked block is still there")
	}
	if err := ForgetPeer(path, "ab12cd"); err != nil {
		t.Errorf("forgetting an absent block = %v, want nil", err)
	}
}
