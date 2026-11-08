package wgconf

import (
	"strings"
	"testing"

	"amneziawg-web-ui/web-ui/api"
)

func fixture(p *api.ObfuscationParams) (*api.Server, *api.Client) {
	srv := &api.Server{
		ID: "abc123", Name: "srv", Interface: "wg-abc123", Port: 54844,
		Subnet: "10.0.1.0/24", ServerIP: "10.0.1.1", MTU: 1280,
		Endpoint: "vpn.example", ServerPublicKey: "SRVPUB",
		ObfuscationEnabled: true, ObfuscationParams: p,
		DNS: []string{"8.8.8.8", "1.1.1.1"},
	}
	cl := &api.Client{
		ID: "c1", Name: "client", ServerID: srv.ID, ClientIP: "10.0.1.2",
		ClientPrivateKey: "CPRIV", ClientPublicKey: "CPUB", PresharedKey: "PSK",
		ObfuscationEnabled: true, ObfuscationParams: p,
	}
	return srv, cl
}

func TestAwg31FlagsOnInClientConf(t *testing.T) {
	p := &api.ObfuscationParams{S1: 50, S2: 60, S3: 20, S4: 16, MTU: 1280,
		HeaderProtectionKey: "KEY", RandomTrailers: true, DisableCookies: true}
	srv, cl := fixture(p)

	conf := ClientConf(srv, cl, "vpn.example", false)
	if !strings.Contains(conf, "RandomTrailers = on\n") || !strings.Contains(conf, "DisableCookies = on\n") {
		t.Fatalf("client conf missing 3.1 switches:\n%s", conf)
	}
	if !strings.Contains(conf, "HeaderProtectionKey = KEY\n") {
		t.Fatalf("client conf missing the header protection key:\n%s", conf)
	}
	if !strings.Contains(conf, "Endpoint = vpn.example:54844\n") || !strings.Contains(conf, "AllowedIPs = "+DefaultAllowedIPs+"\n") {
		t.Fatalf("client conf peer section wrong:\n%s", conf)
	}
}

// The Endpoint line is bracketed when the host is an IPv6 literal: without
// the brackets, "2001:db8::1:54844" is a different - and unusable - address,
// which is why a client config generated from a v6 endpoint never connected.
// The comment line above it carries the same value, so an operator reading
// the file sees what the client was told.
func TestClientConfBracketsAnIPv6Endpoint(t *testing.T) {
	srv, cl := fixture(nil)

	conf := ClientConf(srv, cl, "2001:db8::1", true)
	if !strings.Contains(conf, "\nEndpoint = [2001:db8::1]:54844\n") {
		t.Fatalf("an IPv6 endpoint is not bracketed:\n%s", conf)
	}
	if !strings.Contains(conf, "# Server Endpoint: [2001:db8::1]:54844\n") {
		t.Fatalf("the endpoint comment disagrees with the peer line:\n%s", conf)
	}

	// A name and an IPv4 address stay as they are: brackets are for literals.
	for _, endpoint := range []string{"vpn.example.com", "203.0.113.5"} {
		plain := ClientConf(srv, cl, endpoint, false)
		if !strings.Contains(plain, "\nEndpoint = "+endpoint+":54844\n") {
			t.Fatalf("endpoint %q was rewritten:\n%s", endpoint, plain)
		}
	}
}

func TestAwg31FlagsOffAreOmitted(t *testing.T) {
	p := &api.ObfuscationParams{S1: 50, S2: 60, S3: 20, S4: 16, MTU: 1280, HeaderProtectionKey: "KEY"}
	srv, cl := fixture(p)

	conf := ClientConf(srv, cl, "vpn.example", false)
	if strings.Contains(conf, "RandomTrailers") || strings.Contains(conf, "DisableCookies") {
		t.Fatalf("off flags leaked into conf:\n%s", conf)
	}
}

// The server's [Interface] and the client's carry the same parameter lines:
// obfuscation only works when both ends were issued the same values.
func TestServerAndClientAgreeOnObfuscationLines(t *testing.T) {
	p := &api.ObfuscationParams{Jc: 4, Jmin: 8, Jmax: 80, S1: 50, S2: 60, S3: 20, S4: 16,
		H1: 1, H2: 2, H3: 3, H4: 4, HeaderProtectionKey: "KEY", RandomTrailers: true}
	srv, cl := fixture(p)

	server := ServerInterface{PrivateKey: "SPRIV", Address: "10.0.1.1/24", ListenPort: 54844, MTU: 1280, Obfuscation: p}.Render()
	client := ClientConf(srv, cl, "vpn.example", false)

	for _, line := range []string{"Jc = 4", "S1 = 50", "H4 = 4", "HeaderProtectionKey = KEY", "RandomTrailers = on"} {
		if !strings.Contains(server, line+"\n") || !strings.Contains(client, line+"\n") {
			t.Errorf("%q missing from one side:\n--- server\n%s\n--- client\n%s", line, server, client)
		}
	}
}

// Without I1 nothing signs the first packet, so the rest are not written.
func TestISettingsHangOffI1(t *testing.T) {
	srv, cl := fixture(nil)
	cl.ObfuscationEnabled = false
	cl.ApplyISettings = true
	cl.ISettings = map[string]string{"i2": "<r 20>"}

	if conf := ClientConf(srv, cl, "vpn.example", false); strings.Contains(conf, "I2 = ") {
		t.Errorf("I2 written without I1:\n%s", conf)
	}

	cl.ISettings["i1"] = "<b 0xab>"
	conf := ClientConf(srv, cl, "vpn.example", false)
	if !strings.Contains(conf, "I1 = <b 0xab>\n") || !strings.Contains(conf, "I2 = <r 20>\n") {
		t.Errorf("I-settings missing:\n%s", conf)
	}
}

func TestMergeISettingsLayersOverrides(t *testing.T) {
	got := MergeISettings(map[string]string{"i1": "stored", "i2": "stored2"}, map[string]string{"i1": "new", "i2": ""})
	if got["i1"] != "new" || got["i2"] != "stored2" || got["i3"] != DefaultI3 {
		t.Errorf("MergeISettings = %v", got)
	}
	if got := MergeISettings(); got["i1"] != DefaultI1 {
		t.Errorf("no layers should yield the defaults, got %v", got)
	}
}

// A dual-stack server puts both addresses on one Address line; a server
// without an IPv6 subnet keeps the single address it always had.
func TestServerInterfaceWritesBothAddresses(t *testing.T) {
	iface := ServerInterface{PrivateKey: "PRIV", Address: "10.0.1.1/24", AddressV6: "fd42:ab12::1/64", ListenPort: 54844, MTU: 1280}
	if conf := iface.Render(); !strings.Contains(conf, "Address = 10.0.1.1/24, fd42:ab12::1/64\n") {
		t.Errorf("dual-stack Address line missing:\n%s", conf)
	}

	iface.AddressV6 = ""
	if conf := iface.Render(); !strings.Contains(conf, "Address = 10.0.1.1/24\n") {
		t.Errorf("IPv4-only Address line wrong:\n%s", conf)
	}
}

// The client gets an address on both stacks, and the server is told to route
// both of them to it - otherwise the ::/0 the client routes into the tunnel
// would be answered by nobody.
func TestClientConfAndPeerRoutesCoverBothStacks(t *testing.T) {
	srv, cl := fixture(nil)
	srv.SubnetV6, srv.ServerIPV6 = "fd42:ab12::/64", "fd42:ab12::1"
	cl.ClientIPV6 = "fd42:ab12::2"

	conf := ClientConf(srv, cl, "vpn.example", false)
	if !strings.Contains(conf, "Address = 10.0.1.2/32, fd42:ab12::2/128\n") {
		t.Errorf("client Address line lacks the IPv6 address:\n%s", conf)
	}
	if got := PeerAllowedIPs(cl); got != "10.0.1.2/32, fd42:ab12::2/128" {
		t.Errorf("PeerAllowedIPs = %q", got)
	}

	cl.ClientIPV6 = ""
	if got := PeerAllowedIPs(cl); got != "10.0.1.2/32" {
		t.Errorf("PeerAllowedIPs without IPv6 = %q", got)
	}
}
