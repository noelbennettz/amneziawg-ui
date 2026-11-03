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
