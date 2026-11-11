package e2e

import (
	"slices"
	"strings"
	"testing"
	"time"

	"amneziawg-web-ui/web-ui/api"
)

func TestCreateServer(t *testing.T) {
	a := startApp(t)

	// Open the "Create New VPN Server" accordion. It comes up in the simple
	// mode: a name, a port, and everything else generated on submit.
	a.tap("form.toggle")
	a.shot("02-form")

	// Tick and untick "Advanced settings" first: the rows it appends to the form
	// and the obfuscation block it reveals have to survive the round trip, and
	// the page-error check at the end covers a panic on the way.
	a.tap("form.advanced")
	a.shot("02-advanced")
	a.scroll(750, 500, 3000)
	a.shot("02-advanced-bottom")
	a.scroll(750, 500, -3000)
	a.tap("form.advanced")
	a.shot("02-simple")

	// The name is the only thing the simple mode has no default for.
	a.tapAndType("form.name", "E2E Server")
	a.shot("03-named")

	a.tap("form.create")

	eventually(t, 60*time.Second, "the server never showed up in /api/servers", func() bool {
		return len(getJSON[[]api.Server](t, "/api/servers")) == 1
	})

	server := getJSON[[]api.Server](t, "/api/servers")[0]
	if server.Name != "E2E Server" {
		t.Errorf("name = %q, want %q", server.Name, "E2E Server")
	}

	// Everything below was generated rather than typed, and the two settings the
	// container was started with - DEFAULT_SUBNET and DEFAULT_DNS - are what the
	// form is supposed to start from. They are read back from the status call
	// rather than hardcoded, so the assertion cannot pass by both sides being
	// the built-in default; the values themselves are checked below.
	status := getJSON[api.SystemStatus](t, "/api/system/status")
	if status.Environment.DefaultSubnet == "" {
		t.Fatal("the status call reports no default subnet")
	}
	if server.Subnet != status.Environment.DefaultSubnet {
		t.Errorf("subnet = %q, want DEFAULT_SUBNET %q", server.Subnet, status.Environment.DefaultSubnet)
	}
	// MTU is generated too, from DEFAULT_MTU. It is a top-level field on the
	// server and a separate one inside the obfuscation parameters, so this is
	// also the check that the two agree.
	if server.MTU != status.Environment.DefaultMTU {
		t.Errorf("MTU = %d, want DEFAULT_MTU %d", server.MTU, status.Environment.DefaultMTU)
	}
	if params := server.ObfuscationParams; params != nil && params.MTU != 0 && params.MTU != server.MTU {
		t.Errorf("obfuscation MTU = %d, server MTU = %d", params.MTU, server.MTU)
	}
	// The IPv6 half is generated too: a free ULA /64 and the server's address
	// in it. Without it the ::/0 every client routes into the tunnel would be
	// blackholed, which is the bug this covers.
	if !strings.HasPrefix(server.SubnetV6, "fd42:") || !strings.HasSuffix(server.SubnetV6, "::/64") {
		t.Errorf("subnet_v6 = %q, want a generated ULA /64", server.SubnetV6)
	}
	if !strings.HasPrefix(server.ServerIPV6, strings.TrimSuffix(server.SubnetV6, "/64")) {
		t.Errorf("server_ipv6 = %q, want an address in %s", server.ServerIPV6, server.SubnetV6)
	}
	wantDNS := strings.Split(status.Environment.DefaultDNS, ",")
	if len(wantDNS) == 0 || wantDNS[0] == "" {
		t.Fatal("the status call reports no default DNS")
	}
	if !slices.Equal(server.DNS, wantDNS) {
		t.Errorf("dns = %v, want DEFAULT_DNS %v", server.DNS, wantDNS)
	}
	if server.Endpoint == "" {
		t.Error("the endpoint is empty")
	}
	if !server.ObfuscationEnabled {
		t.Error("obfuscation is off")
	}
	params := server.ObfuscationParams
	if params == nil {
		t.Fatal("the server has no obfuscation parameters")
	}
	if !params.RandomTrailers {
		t.Error("RandomTrailers is off")
	}
	if params.HeaderProtectionKey == "" {
		t.Error("HeaderProtectionKey is empty")
	}

	// The MTU is the operator's DEFAULT_MTU, not a number the form carries of
	// its own - the simple mode has no field to show it in, so this is the only
	// place the setting can be seen to apply.
	if server.MTU != status.Environment.DefaultMTU {
		t.Errorf("mtu = %d, want DEFAULT_MTU %d", server.MTU, status.Environment.DefaultMTU)
	}

	paddings := map[string]int{"S1": params.S1, "S2": params.S2, "S3": params.S3, "S4": params.S4}
	for key, value := range paddings {
		if value < 12 {
			t.Errorf("%s = %d, must be >= 12 for AWG 3.x header protection", key, value)
		}
	}
	if params.S1+56 == params.S2 {
		t.Errorf("S1 + 56 == S2 (%d, %d)", params.S1, params.S2)
	}

	// The shape the AmneziaWG docs recommend for header protection with random
	// trailers: one padding for all four message types, headers left standard.
	if params.S1 != params.S2 || params.S1 != params.S3 || params.S1 != params.S4 {
		t.Errorf("paddings differ: S1..S4 = %d %d %d %d", params.S1, params.S2, params.S3, params.S4)
	}
	if headers := []int{params.H1, params.H2, params.H3, params.H4}; !slices.Equal(headers, []int{1, 2, 3, 4}) {
		t.Errorf("H1..H4 = %v, want [1 2 3 4]", headers)
	}

	// And a full transport packet still fits a standard 1500-byte path: S4 rides
	// on every one of them.
	if size := server.MTU + 60 + params.S4; size > 1500 {
		t.Errorf("mtu + 60 + S4 = %d, over 1500", size)
	}

	time.Sleep(4 * time.Second)
	a.shot("05-created")
	a.noPageErrors()
}
