package netutil

import "testing"

func TestNextFreeIPSkipsNetworkBroadcastAndUsed(t *testing.T) {
	used := map[string]bool{"10.0.1.1": true, "10.0.1.2": true}
	if got := NextFreeIP("10.0.1.0/24", used); got != "10.0.1.3" {
		t.Errorf("NextFreeIP = %q, want 10.0.1.3", got)
	}

	// A /30 has exactly two hosts; both taken means full.
	full := map[string]bool{"10.0.1.1": true, "10.0.1.2": true}
	if got := NextFreeIP("10.0.1.0/30", full); got != "" {
		t.Errorf("full subnet handed out %q", got)
	}

	if got := NextFreeIP("10.0.1.0", nil); got != "" {
		t.Errorf("subnet without a prefix handed out %q", got)
	}
}

func TestSplitCIDRAndServerIP(t *testing.T) {
	if n, p := SplitCIDR("10.1.2.0/16"); n != "10.1.2.0" || p != "16" {
		t.Errorf("SplitCIDR = %q/%q", n, p)
	}
	if n, p := SplitCIDR("10.1.2.0"); n != "10.1.2.0" || p != "24" {
		t.Errorf("SplitCIDR without prefix = %q/%q", n, p)
	}
	if got := ServerIP("10.1.2.0"); got != "10.1.2.1" {
		t.Errorf("ServerIP = %q", got)
	}
	if n, p := SplitCIDR("fd42:ab12::/64"); n != "fd42:ab12::" || p != "64" {
		t.Errorf("SplitCIDR(v6) = %q/%q", n, p)
	}
}

func TestServerIPV6IsTheFirstHost(t *testing.T) {
	if got := ServerIPV6("fd42:ab12::"); got != "fd42:ab12::1" {
		t.Errorf("ServerIPV6 = %q, want fd42:ab12::1", got)
	}
	if got := ServerIPV6("10.0.1.0"); got != "" {
		t.Errorf("ServerIPV6(IPv4) = %q, want it rejected", got)
	}
	if got := ServerIPV6("garbage"); got != "" {
		t.Errorf("ServerIPV6(garbage) = %q, want it rejected", got)
	}
}

func TestNextFreeIPv6SkipsUsed(t *testing.T) {
	used := map[string]bool{"fd42:ab12::1": true, "fd42:ab12::2": true}
	if got := NextFreeIPv6("fd42:ab12::/64", used); got != "fd42:ab12::3" {
		t.Errorf("NextFreeIPv6 = %q, want fd42:ab12::3", got)
	}

	// A /126 has four addresses: ::1 and ::2 are taken, ::3 is the last one
	// left, ::4 is outside the block.
	small := map[string]bool{"fd42:ab12::1": true, "fd42:ab12::2": true}
	if got := NextFreeIPv6("fd42:ab12::/126", small); got != "fd42:ab12::3" {
		t.Errorf("NextFreeIPv6(/126) = %q, want fd42:ab12::3", got)
	}
	full := map[string]bool{"fd42:ab12::1": true, "fd42:ab12::2": true, "fd42:ab12::3": true}
	if got := NextFreeIPv6("fd42:ab12::/126", full); got != "" {
		t.Errorf("full IPv6 subnet handed out %q", got)
	}

	if got := NextFreeIPv6("10.0.1.0/24", nil); got != "" {
		t.Errorf("IPv4 subnet handed out IPv6 %q", got)
	}
}
