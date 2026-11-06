package manager

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"amneziawg-web-ui/web-ui/api"
)

// A server created without an IPv6 subnet gets a free ULA /64 named after its
// ID, so the addresses in its configs say which server they belong to, and two
// servers on one host never share a /64.
func TestCreateServerGivesItAFreeUlaSubnet(t *testing.T) {
	m, _ := newTestManager(t)

	first, err := m.CreateServer(api.CreateServerRequest{Name: "first", Port: 54846, Subnet: "10.0.2.0/24", MTU: 1280})
	if err != nil {
		t.Fatal(err)
	}
	// The /64 is fd42 plus the first four hex digits of the ID. Spelled the
	// way net.IP spells an address: an ID starting 07cc gives fd42:7cc::, not
	// fd42:07cc::, because canonical IPv6 text drops a group's leading zeros -
	// and the server's own address is built from that same text. Expecting
	// ID[:4] verbatim here made the test fail one run in sixteen, whenever the
	// random ID happened to start with a zero.
	group, err := strconv.ParseUint(first.ID[:4], 16, 16)
	if err != nil {
		t.Fatalf("server ID %q does not start with hex digits: %v", first.ID, err)
	}
	if want := fmt.Sprintf("fd42:%x::/64", group); first.SubnetV6 != want {
		t.Errorf("SubnetV6 = %q, want %q", first.SubnetV6, want)
	}
	if want := fmt.Sprintf("fd42:%x::1", group); first.ServerIPV6 != want {
		t.Errorf("ServerIPV6 = %q, want %q", first.ServerIPV6, want)
	}

	conf, err := os.ReadFile(first.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conf), "Address = 10.0.2.1/24, "+first.ServerIPV6+"/64\n") {
		t.Errorf("server .conf is not dual-stack:\n%s", conf)
	}

	second, err := m.CreateServer(api.CreateServerRequest{Name: "second", Port: 54847, Subnet: "10.0.3.0/24", MTU: 1280})
	if err != nil {
		t.Fatal(err)
	}
	if second.SubnetV6 == first.SubnetV6 {
		t.Errorf("both servers got the same IPv6 subnet: %s", second.SubnetV6)
	}
}

// An operator who names the IPv6 subnet gets exactly that one; one that clashes
// with another server is a conflict, and one that is not a ULA with room for a
// client is a bad request.
func TestCreateServerChecksTheIPv6Subnet(t *testing.T) {
	m, _ := newTestManager(t)

	srv, err := m.CreateServer(api.CreateServerRequest{Name: "v6", Port: 54846, Subnet: "10.0.2.0/24", SubnetV6: "fd99:1::/64", MTU: 1280})
	if err != nil {
		t.Fatal(err)
	}
	if srv.SubnetV6 != "fd99:1::/64" || srv.ServerIPV6 != "fd99:1::1" {
		t.Errorf("server = %q / %q, want fd99:1::/64 / fd99:1::1", srv.SubnetV6, srv.ServerIPV6)
	}

	_, err = m.CreateServer(api.CreateServerRequest{Name: "clash", Port: 54847, Subnet: "10.0.3.0/24", SubnetV6: "fd99:1::/64", MTU: 1280})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("overlapping IPv6 subnet: want ErrConflict, got %v", err)
	}

	for _, bad := range []string{"2001:db8::/64", "fd99:2::/121", "10.0.4.0/24"} {
		_, err := m.CreateServer(api.CreateServerRequest{Name: "bad", Port: 54848, Subnet: "10.0.4.0/24", SubnetV6: bad, MTU: 1280})
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("IPv6 subnet %q: want ErrInvalid, got %v", bad, err)
		}
	}
}

// Each client is handed an address on both stacks, the server routes both of
// them to it, and a deleted client's address goes back into the pool.
func TestAddClientGetsAnAddressOnBothStacks(t *testing.T) {
	m, _ := newTestManager(t)
	srv, err := m.CreateServer(api.CreateServerRequest{Name: "v6", Port: 54846, Subnet: "10.0.2.0/24", MTU: 1280})
	if err != nil {
		t.Fatal(err)
	}

	client, conf, err := m.AddClient(srv.ID, api.AddClientRequest{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if client.ClientIPV6 != srv.ServerIPV6[:len(srv.ServerIPV6)-1]+"2" {
		t.Errorf("ClientIPV6 = %q, want the first address after the server's", client.ClientIPV6)
	}
	if !strings.Contains(conf, "Address = "+client.ClientIP+"/32, "+client.ClientIPV6+"/128\n") {
		t.Errorf("client .conf is not dual-stack:\n%s", conf)
	}

	confOnDisk, err := os.ReadFile(srv.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(confOnDisk), "AllowedIPs = "+client.ClientIP+"/32, "+client.ClientIPV6+"/128\n") {
		t.Errorf("server does not route both client addresses:\n%s", confOnDisk)
	}

	if err := m.DeleteClient(srv.ID, client.ID); err != nil {
		t.Fatal(err)
	}
	next, _, err := m.AddClient(srv.ID, api.AddClientRequest{Name: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if next.ClientIPV6 != client.ClientIPV6 {
		t.Errorf("freed IPv6 address not reused: %q, want %q", next.ClientIPV6, client.ClientIPV6)
	}
	if next.ClientIP != client.ClientIP {
		t.Errorf("freed IPv4 address not reused: %q, want %q", next.ClientIP, client.ClientIP)
	}
}

// A .conf written before IPv6 support gets the addresses the config already
// carries, and a second pass leaves it alone.
func TestReconcileIPv6ConfFixesAnOldFile(t *testing.T) {
	m, _ := newTestManager(t)
	srv := &m.cfg.Servers[0]
	srv.SubnetV6, srv.ServerIPV6 = "fd42:ab12::/64", "fd42:ab12::1"
	srv.Clients = []api.Client{{ID: "c1", Name: "alice", ServerID: srv.ID, ClientIP: "10.0.1.2", ClientIPV6: "fd42:ab12::2"}}

	old := "[Interface]\nPrivateKey = PRIV\nAddress = 10.0.1.1/24\nListenPort = 54844\n\n" +
		"# Client: alice [id:c1]\n[Peer]\nPublicKey = CPUB\nPresharedKey = PSK\nAllowedIPs = 10.0.1.2/32\n"
	if err := os.WriteFile(srv.ConfigPath, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	m.reconcileIPv6Conf()

	data, err := os.ReadFile(srv.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Address = 10.0.1.1/24, fd42:ab12::1/64\n",
		"AllowedIPs = 10.0.1.2/32, fd42:ab12::2/128\n",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("conf missing %q:\n%s", want, data)
		}
	}

	m.reconcileIPv6Conf()
	again, err := os.ReadFile(srv.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(data) {
		t.Errorf("second reconcile rewrote the file:\n%s", again)
	}
}
