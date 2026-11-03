package api

import (
	"fmt"
	"net"
	"strconv"
)

// MaxSubnetPrefix is the longest prefix a server's IPv4 subnet may have: a /30
// is the smallest block with room for the server and one client between its
// network and broadcast addresses.
const MaxSubnetPrefix = 30

// MaxSubnetPrefixV6 is the longest prefix a server's IPv6 subnet may have: a
// /120 leaves 8 host bits, room for the server and 254 clients.
const MaxSubnetPrefixV6 = 120

// ULAv6Group is the 16-bit group every automatically assigned IPv6 subnet
// starts from: fd42::/16 sits inside fc00::/7, the private block RFC 4193 sets
// aside, so an address handed to a client can never collide with a global one
// the host itself holds.
const ULAv6Group = "fd42"

// ParseSubnet reads a server's subnet: an IPv4 CIDR with room for at least
// one client. The result is the network itself, so "10.0.0.5/24" comes back
// as 10.0.0.0/24.
func ParseSubnet(subnet string) (*net.IPNet, error) {
	ip, network, err := net.ParseCIDR(subnet)
	if err != nil || ip.To4() == nil {
		return nil, fmt.Errorf("subnet %q is not an IPv4 CIDR such as 10.0.0.0/24", subnet)
	}
	if ones, _ := network.Mask.Size(); ones > MaxSubnetPrefix {
		return nil, fmt.Errorf("subnet %q is too small: the prefix can be at most /%d", subnet, MaxSubnetPrefix)
	}
	return network, nil
}

// ParseSubnetV6 reads a server's IPv6 subnet: a ULA CIDR with room for at
// least one client. The result is the network itself, so "fd42:ab12::5/64"
// comes back as fd42:ab12::/64.
//
// Only ULA (fc00::/7) is accepted. The panel masquerades this subnet onto the
// host's own uplink - that is what makes NAT6 work - so a global prefix, which
// the host's router might already route somewhere else, has no business here.
func ParseSubnetV6(subnet string) (*net.IPNet, error) {
	ip, network, err := net.ParseCIDR(subnet)
	if err != nil || ip.To4() != nil || ip.To16() == nil {
		return nil, fmt.Errorf("IPv6 subnet %q is not a CIDR such as %s:ab12::/64", subnet, ULAv6Group)
	}
	if !isULA(network.IP) {
		return nil, fmt.Errorf("IPv6 subnet %q is not a ULA block: it must start with fc00::/7 (e.g. %s:ab12::/64)", subnet, ULAv6Group)
	}
	if ones, _ := network.Mask.Size(); ones > MaxSubnetPrefixV6 {
		return nil, fmt.Errorf("IPv6 subnet %q is too small: the prefix can be at most /%d", subnet, MaxSubnetPrefixV6)
	}
	return network, nil
}

// isULA reports whether ip is inside fc00::/7, the block RFC 4193 reserves for
// unique local addresses. The first seven bits are 1111 110.
func isULA(ip net.IP) bool {
	v6 := ip.To16()
	return v6 != nil && ip.To4() == nil && v6[0]&0xfe == 0xfc
}

// NextFreeSubnetV6 returns the first ULA /64 that overlaps none of the taken
// subnets, starting from the group the seed stands for - a server ID, so a
// server's subnet is recognisably its own. overlaps is the caller's check
// against everything already in use; the empty string means the whole 16-bit
// group space is taken, which no real deployment reaches.
//
// Both the backend and the create form call this, so a subnet the form offered
// is the one the backend hands out.
func NextFreeSubnetV6(seed string, overlaps func(string) bool) string {
	start := int(seedGroup(seed))
	for i := range 1 << 16 {
		candidate := fmt.Sprintf("%s:%x::/64", ULAv6Group, uint16(start+i))
		if !overlaps(candidate) {
			return candidate
		}
	}
	return ""
}

// seedGroup turns a seed into the 16-bit group that starts the search: the
// first four hex digits it contains, so server ID "ab12cd" gives fd42:ab12.
// A seed without hex digits at all starts at fd42:0.
func seedGroup(seed string) uint16 {
	digits := make([]byte, 0, 4)
	for i := 0; i < len(seed) && len(digits) < 4; i++ {
		if c := seed[i]; '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' {
			digits = append(digits, c)
		}
	}
	if len(digits) == 0 {
		return 0
	}
	value, err := strconv.ParseUint(string(digits), 16, 16)
	if err != nil {
		return 0
	}
	return uint16(value)
}

// SubnetsOverlap reports whether two subnets share any address. Two servers
// on overlapping subnets would route the same client addresses to two
// interfaces, so one of them silently stops receiving traffic. The two
// families never overlap each other, which is what lets the same check guard
// IPv4 and IPv6 subnets. A subnet that does not parse overlaps nothing;
// ParseSubnet and ParseSubnetV6 are what report it.
func SubnetsOverlap(a, b string) bool {
	na, nb := parseEither(a), parseEither(b)
	if na == nil || nb == nil {
		return false
	}
	return na.Contains(nb.IP) || nb.Contains(na.IP)
}

// parseEither reads a subnet of either family, or nil when it is neither.
func parseEither(subnet string) *net.IPNet {
	if network, err := ParseSubnet(subnet); err == nil {
		return network
	}
	if network, err := ParseSubnetV6(subnet); err == nil {
		return network
	}
	return nil
}
