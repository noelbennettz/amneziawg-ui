package wgconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amneziawg-web-ui/web-ui/api"
)

func newServerConf(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wg-s1.conf")
	iface := ServerInterface{
		PrivateKey: "PRIV", Address: "10.0.1.1/24", ListenPort: 54844, MTU: 1280,
		Obfuscation: &api.ObfuscationParams{S1: 50},
	}
	if err := WriteServerConf(path, iface); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParkAndRestorePeerRoundTrip(t *testing.T) {
	path := newServerConf(t)
	alice := &api.Client{ID: "aaa", Name: "alice", ClientPublicKey: "APUB", PresharedKey: "APSK"}
	bob := &api.Client{ID: "bbb", Name: "bob", ClientPublicKey: "BPUB", PresharedKey: "BPSK"}
	for _, c := range []*api.Client{alice, bob} {
		if err := AppendPeer(path, c, "10.0.1.2/32"); err != nil {
			t.Fatal(err)
		}
	}

	if err := ParkPeer(path, alice); err != nil {
		t.Fatal(err)
	}
	conf := read(t, path)
	if strings.Contains(conf, "APUB") || !strings.Contains(conf, "BPUB") || !strings.Contains(conf, "S1 = 50") {
		t.Fatalf("after park:\n%s", conf)
	}
	parked := filepath.Join(SuspendedDir(path), "aaa.conf")
	if _, err := os.Stat(parked); err != nil {
		t.Fatalf("parked block missing: %v", err)
	}

	alice.Name = "alice-renamed"
	if err := RestorePeer(path, alice); err != nil {
		t.Fatal(err)
	}
	conf = read(t, path)
	if !strings.Contains(conf, "APUB") || !strings.Contains(conf, PeerMarker("alice-renamed", "aaa")) {
		t.Fatalf("after restore:\n%s", conf)
	}
	if strings.Count(conf, "[Peer]") != 2 {
		t.Fatalf("peer count after round trip:\n%s", conf)
	}
	if _, err := os.Stat(parked); !os.IsNotExist(err) {
		t.Errorf("parked copy left behind (err %v)", err)
	}
}

func TestRestoringAnUnparkedPeerFails(t *testing.T) {
	path := newServerConf(t)
	if err := RestorePeer(path, &api.Client{ID: "nope"}); err == nil {
		t.Error("restoring a peer that was never parked must fail")
	}
}

// A parked file is read back by client ID, and IDs are six hex characters: a
// block that belongs to somebody else must be refused, not relabelled onto
// this client's name. Otherwise whoever holds the other private key is a peer
// of this server, and this client's own traffic goes to the wrong place.
func TestRestoreRefusesAParkedBlockWithSomebodyElsesKey(t *testing.T) {
	path := newServerConf(t)
	alice := &api.Client{ID: "aaa", Name: "alice", ClientPublicKey: "APUB", PresharedKey: "APSK", ClientIP: "10.0.1.2"}
	stranger := &api.Client{ID: "aaa", Name: "alice", ClientPublicKey: "STRANGER", PresharedKey: "SPKS", ClientIP: "10.0.1.3"}
	if err := AppendPeer(path, stranger, "10.0.1.3/32"); err != nil {
		t.Fatal(err)
	}
	if err := ParkPeer(path, stranger); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(SuspendedDir(path), "aaa.conf")
	before := read(t, parked)

	err := RestorePeer(path, alice)
	if err == nil {
		t.Fatal("a parked block with another public key was accepted")
	}
	if !strings.Contains(err.Error(), "STRANGER") {
		t.Errorf("the error should name the key it found, got %v", err)
	}
	if read(t, parked) != before {
		t.Error("the refused block should be left where it was")
	}
	if strings.Contains(read(t, path), "STRANGER") {
		t.Error("the stranger's key ended up in the live config")
	}
}

// The parked block is stale in the parts the model owns: a client renamed and
// re-addressed while it was out comes back as it is now, not as it was.
func TestRestoreTakesKeysAndAddressesFromTheClient(t *testing.T) {
	path := newServerConf(t)
	alice := &api.Client{ID: "aaa", Name: "alice", ClientPublicKey: "APUB", PresharedKey: "APSK", ClientIP: "10.0.1.2"}
	if err := AppendPeer(path, alice, "10.0.1.2/32"); err != nil {
		t.Fatal(err)
	}
	if err := ParkPeer(path, alice); err != nil {
		t.Fatal(err)
	}

	alice.Name = "alice-now"
	alice.PresharedKey = "NEWPSK"
	alice.ClientIP = "10.0.1.9"
	alice.ClientIPV6 = "fd42:ab12::9"
	if err := RestorePeer(path, alice); err != nil {
		t.Fatal(err)
	}

	conf := read(t, path)
	for _, want := range []string{"PresharedKey = NEWPSK", "AllowedIPs = 10.0.1.9/32, fd42:ab12::9/128", PeerMarker("alice-now", "aaa")} {
		if !strings.Contains(conf, want) {
			t.Errorf("restored config is missing %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "APSK") {
		t.Errorf("the old preshared key is still in the peer block:\n%s", conf)
	}
}

func TestRemovePeerOfAClientWithoutABlockIsANoOp(t *testing.T) {
	path := newServerConf(t)
	before := read(t, path)
	block, err := RemovePeer(path, &api.Client{ID: "ghost"})
	if err != nil || block != nil {
		t.Errorf("RemovePeer = %v, %v", block, err)
	}
	if read(t, path) != before {
		t.Error("the file was rewritten for nothing")
	}
}
