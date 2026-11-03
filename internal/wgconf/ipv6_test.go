package wgconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amneziawg-web-ui/web-ui/api"
)

// oldConf is what a release without IPv6 support left on disk: one IPv4
// address in the [Interface] section and one IPv4 route per peer.
const oldConf = `[Interface]
PrivateKey = PRIV
Address = 10.0.1.1/24
ListenPort = 54844
SaveConfig = false
MTU = 1280

# Client: alice [id:c1]
[Peer]
PublicKey = CPUB
PresharedKey = PSK
AllowedIPs = 10.0.1.2/32
`

func writeConf(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wg-test.conf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEnsureInterfaceAddressAddsOnlyTheMissingAddress(t *testing.T) {
	path := writeConf(t, oldConf)

	if err := EnsureInterfaceAddress(path, "fd42:ab12::1/64"); err != nil {
		t.Fatal(err)
	}
	got := readString(t, path)
	if !strings.Contains(got, "Address = 10.0.1.1/24, fd42:ab12::1/64\n") {
		t.Fatalf("IPv6 address not added:\n%s", got)
	}

	// Running it again must not append the address a second time.
	if err := EnsureInterfaceAddress(path, "fd42:ab12::1/64"); err != nil {
		t.Fatal(err)
	}
	if again := readString(t, path); again != got {
		t.Fatalf("second run changed the file:\n%s", again)
	}
}

func TestEnsurePeerAllowedIPsAddsTheIPv6Route(t *testing.T) {
	path := writeConf(t, oldConf)
	client := &api.Client{ID: "c1", Name: "alice", ClientIP: "10.0.1.2", ClientIPV6: "fd42:ab12::2"}

	if err := EnsurePeerAllowedIPs(path, client, PeerAllowedIPs(client)); err != nil {
		t.Fatal(err)
	}
	got := readString(t, path)
	if !strings.Contains(got, "AllowedIPs = 10.0.1.2/32, fd42:ab12::2/128\n") {
		t.Fatalf("IPv6 route not added to the peer block:\n%s", got)
	}
	// The [Interface] Address line is not this function's business.
	if !strings.Contains(got, "Address = 10.0.1.1/24\n") {
		t.Fatalf("a peer edit rewrote the interface section:\n%s", got)
	}
}

// A suspended client's block is parked beside the .conf and goes back in when
// it is activated, so the parked copy has to carry the IPv6 route too.
func TestEnsurePeerAllowedIPsReachesASuspendedClient(t *testing.T) {
	path := writeConf(t, oldConf)
	client := &api.Client{ID: "c1", Name: "alice", ClientIP: "10.0.1.2", ClientIPV6: "fd42:ab12::2"}

	block, err := RemovePeer(path, client)
	if err != nil {
		t.Fatal(err)
	}
	dir := SuspendedDir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(dir, "c1.conf")
	if err := os.WriteFile(parked, []byte(strings.Join(block, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := EnsurePeerAllowedIPs(path, client, PeerAllowedIPs(client)); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, parked); !strings.Contains(got, "AllowedIPs = 10.0.1.2/32, fd42:ab12::2/128\n") {
		t.Fatalf("parked block still routes IPv4 only:\n%s", got)
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
