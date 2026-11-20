package api

import "testing"

func TestParseSubnetReturnsTheNetwork(t *testing.T) {
	n, err := ParseSubnet("10.0.0.5/24")
	if err != nil || n.String() != "10.0.0.0/24" {
		t.Fatalf("ParseSubnet = %v, %v; want 10.0.0.0/24", n, err)
	}
	for _, bad := range []string{"", "abc", "10.0.0.0", "10.0.0.0/31", "300.0.0.0/24", "fd00::/64"} {
		if _, err := ParseSubnet(bad); err == nil {
			t.Errorf("ParseSubnet(%q) should fail", bad)
		}
	}
}

func TestSubnetsOverlap(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"10.0.0.0/24", "10.0.0.0/24", true},
		{"10.0.0.0/16", "10.0.5.0/24", true},
		{"10.0.5.0/24", "10.0.0.0/16", true},
		{"10.0.0.0/25", "10.0.0.128/25", false},
		{"10.0.0.0/24", "10.1.0.0/24", false},
		{"10.0.0.0/24", "garbage", false},
		// The same block, the containing one and the one inside it, now on
		// the IPv6 side.
		{"fd42:ab12::/64", "fd42:ab12::/64", true},
		{"fd42::/16", "fd42:ab12::/64", true},
		{"fd42:ab12::/64", "fd42::/16", true},
		{"fd42:ab12::/64", "fd42:ab13::/64", false},
		// The two families are separate address spaces and never overlap.
		{"10.0.0.0/24", "fd42:ab12::/64", false},
		{"fd42:ab12::/64", "10.0.0.0/24", false},
	}
	for _, c := range cases {
		if got := SubnetsOverlap(c.a, c.b); got != c.want {
			t.Errorf("SubnetsOverlap(%s, %s) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestParseSubnetV6ReturnsTheNetwork(t *testing.T) {
	n, err := ParseSubnetV6("fd42:ab12::5/64")
	if err != nil || n.String() != "fd42:ab12::/64" {
		t.Fatalf("ParseSubnetV6 = %v, %v; want fd42:ab12::/64", n, err)
	}

	for _, bad := range []string{
		"", "abc", "fd42:ab12::", "10.0.0.0/24",
		"2001:db8::/64",   // global unicast: NAT6 assumes a private block
		"fd42:ab12::/121", // no room for a client
	} {
		if _, err := ParseSubnetV6(bad); err == nil {
			t.Errorf("ParseSubnetV6(%q) should fail", bad)
		}
	}
}

// A new server's subnet is named after the server, so the addresses in the
// configs say which server they belong to; a taken group moves on to the next.
func TestNextFreeSubnetV6StartsFromTheSeed(t *testing.T) {
	got := NextFreeSubnetV6("ab12cd", func(string) bool { return false })
	if got != "fd42:ab12::/64" {
		t.Errorf("NextFreeSubnetV6 = %q, want fd42:ab12::/64", got)
	}

	taken := map[string]bool{"fd42:ab12::/64": true, "fd42:ab13::/64": true}
	got = NextFreeSubnetV6("ab12cd", func(candidate string) bool { return taken[candidate] })
	if got != "fd42:ab14::/64" {
		t.Errorf("NextFreeSubnetV6 = %q, want fd42:ab14::/64", got)
	}
}

// The group is spelled the way an address is: leading zeros dropped. A seed
// whose first four hex digits start with 0 - one server ID in sixteen - has to
// print fd42:7cc::/64, not fd42:07cc::/64, because ServerIPV6 builds the
// server's own address from the same text and the two have to agree.
func TestNextFreeSubnetV6DropsLeadingZerosInTheGroup(t *testing.T) {
	if got := NextFreeSubnetV6("07cc12", func(string) bool { return false }); got != "fd42:7cc::/64" {
		t.Errorf("NextFreeSubnetV6 = %q, want fd42:7cc::/64", got)
	}
	if got := NextFreeSubnetV6("0000ab", func(string) bool { return false }); got != "fd42:0::/64" {
		t.Errorf("NextFreeSubnetV6 = %q, want fd42:0::/64", got)
	}
}

// A subnet a server cannot safely route is refused before it can put a route
// on the host: 0.0.0.0/0 replaces the default route and cuts the machine off,
// and the reserved blocks belong to the kernel rather than to a tunnel.
func TestParseSubnetRefusesUnsafeBlocks(t *testing.T) {
	for _, subnet := range []string{
		"0.0.0.0/0", "0.0.0.0/8", "127.0.0.0/8", "127.10.0.0/16",
		"169.254.0.0/16", "224.0.0.0/4", "240.0.0.0/4", "10.0.0.0/7",
	} {
		if _, err := ParseSubnet(subnet); err == nil {
			t.Errorf("ParseSubnet(%q) accepted it", subnet)
		}
	}

	// The blocks a deployment actually uses - a /8 included - still pass.
	for _, subnet := range []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "10.9.0.128/25",
	} {
		if _, err := ParseSubnet(subnet); err != nil {
			t.Errorf("ParseSubnet(%q) = %v, want it accepted", subnet, err)
		}
	}
}

// A ULA wider than a site is not one tunnel: it puts an enormous route on the
// interface and overlaps the /64 every server after it is given.
func TestParseSubnetV6RefusesAWiderBlockThanASite(t *testing.T) {
	for _, subnet := range []string{"fd00::/8", "fc00::/7", "fd42::/47"} {
		if _, err := ParseSubnetV6(subnet); err == nil {
			t.Errorf("ParseSubnetV6(%q) accepted it", subnet)
		}
	}
	for _, subnet := range []string{"fd42::/48", "fd42:ab12::/64", "fd42:ab12::/120"} {
		if _, err := ParseSubnetV6(subnet); err != nil {
			t.Errorf("ParseSubnetV6(%q) = %v, want it accepted", subnet, err)
		}
	}
}
