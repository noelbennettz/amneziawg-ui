package api

import (
	"net"
	"testing"
)

// The endpoint field takes all three shapes an operator can legitimately
// mean: a name, an IPv4 address, and an IPv6 address - the last one with or
// without the brackets a .conf file writes it in.
func TestParseEndpointAcceptsEveryHostShape(t *testing.T) {
	cases := []struct{ in, want string }{
		{"vpn.example.com", "vpn.example.com"},
		{"  edge-1.example.com.  ", "edge-1.example.com."},
		{"203.0.113.5", "203.0.113.5"},
		{"2001:db8::1", "2001:db8::1"},
		{"[2001:db8::1]", "2001:db8::1"},
		{"2001:0db8:0000:0000:0000:0000:0000:0001", "2001:db8::1"},
		{"[::ffff:203.0.113.5]", "::ffff:203.0.113.5"},
		{"a", "a"},
	}
	for _, tc := range cases {
		got, err := ParseEndpoint(tc.in)
		if err != nil {
			t.Errorf("ParseEndpoint(%q) = error %v, want %q", tc.in, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseEndpoint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A port, a scheme or a path in this field is a misreading of it, and every
// one of them would come out as a broken Endpoint line: the server's own port
// is appended afterwards.
func TestParseEndpointRejectsWhatIsNotAHost(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"https://vpn.example.com",
		"vpn.example.com:51820",
		"203.0.113.5:51820",
		"[2001:db8::1]:51820",
		"2001:db8::1/64",
		"vpn.example.com/",
		"[2001:db8::1",
		"2001:db8::zz",
		"bad_host.example.com",
		"-edge.example.com",
		"edge-.example.com",
		"a..b",
		".",
	}
	for _, in := range cases {
		if got, err := ParseEndpoint(in); err == nil {
			t.Errorf("ParseEndpoint(%q) = %q, want an error", in, got)
		}
	}
}

// The Endpoint line is where the brackets have to be, and only for IPv6: an
// unbracketed "2001:db8::1:51820" is a different (and empty) address.
func TestEndpointAddressBracketsOnlyIPv6(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"vpn.example.com", "vpn.example.com:54844"},
		{"203.0.113.5", "203.0.113.5:54844"},
		{"2001:db8::1", "[2001:db8::1]:54844"},
		{"[2001:db8::1]", "[2001:db8::1]:54844"},
		{"fd42:7cc::1", "[fd42:7cc::1]:54844"},
	}
	for _, tc := range cases {
		if got := EndpointAddress(tc.host, 54844); got != tc.want {
			t.Errorf("EndpointAddress(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}

	// What it writes has to parse as an address and a port again, which is
	// the whole point of the brackets.
	if host, port, err := net.SplitHostPort(EndpointAddress("2001:db8::1", 54844)); err != nil || host != "2001:db8::1" || port != "54844" {
		t.Fatalf("SplitHostPort(EndpointAddress(...)) = %q, %q, %v", host, port, err)
	}
}

// The canonical spelling is what goes into a client config, so the value that
// is accepted and the value that is written are decided by the same call. Both
// stacks count; a zone is refused, because it names a local interface and the
// client on the other end of the tunnel has no such interface.
func TestCanonicalIPAddressSpellsItTheWayConfigsDo(t *testing.T) {
	cases := []struct{ in, want string }{
		{" 8.8.8.8 ", "8.8.8.8"},
		{"::1", "::1"},
		{"0:0:0:0:0:0:0:1", "::1"},
		{"::ffff:8.8.8.8", "8.8.8.8"},
		{"2606:4700:4700:0:0:0:0:1111", "2606:4700:4700::1111"},
	}
	for _, tc := range cases {
		got, ok := CanonicalIPAddress(tc.in)
		if !ok || got != tc.want {
			t.Errorf("CanonicalIPAddress(%q) = %q, %v; want %q, true", tc.in, got, ok, tc.want)
		}
	}

	for _, in := range []string{"", "dns.google", "8.8.8.8:53", "[::1]", "::1/128", "fe80::1%eth0"} {
		if got, ok := CanonicalIPAddress(in); ok {
			t.Errorf("CanonicalIPAddress(%q) = %q, true; want false", in, got)
		}
	}
}
