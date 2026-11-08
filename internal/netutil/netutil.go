// Package netutil holds the small pieces of address arithmetic the backend
// needs to hand out addresses inside a server's subnet, for both stacks.
package netutil

import (
	"fmt"
	"net"
	"strings"
)

// maxScanV6 caps how many addresses NextFreeIPv6 walks. A /64 holds more
// clients than this panel will ever see, and a walk of one that is somehow
// full has to end.
const maxScanV6 = 1 << 16

// IsIPv4 reports whether s is a literal IPv4 address.
func IsIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

// IsIPv6 reports whether s is a literal IPv6 address. The IPv4-mapped form
// "::ffff:203.0.113.5" counts as IPv4, since that is the address it carries.
func IsIPv6(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil
}

// SplitCIDR separates "10.0.0.0/24" into its network and prefix length. A
// bare network gets the prefix that every default subnet here uses.
func SplitCIDR(subnet string) (network, prefix string) {
	parts := strings.SplitN(subnet, "/", 2)
	if len(parts) > 1 {
		return parts[0], parts[1]
	}
	return parts[0], "24"
}

// ServerIP is the address the server takes in its own subnet: the first host,
// so 10.0.0.0 becomes 10.0.0.1.
func ServerIP(network string) string {
	parts := strings.Split(network, ".")
	if len(parts) == 4 {
		return fmt.Sprintf("%s.%s.%s.1", parts[0], parts[1], parts[2])
	}
	return "10.0.0.1"
}

// NextFreeIP returns the lowest host address in subnet that is not in used,
// or "" when the subnet is full or malformed. The network and broadcast
// addresses are never handed out.
func NextFreeIP(subnet string, used map[string]bool) string {
	if !strings.Contains(subnet, "/") {
		return ""
	}
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return ""
	}

	for ip := cloneIP(ipNet.IP); ipNet.Contains(ip); incrementIP(ip) {
		s := ip.String()
		if s == ipNet.IP.String() {
			continue // network address
		}
		if isBroadcast(ip, ipNet) {
			continue
		}
		if !used[s] {
			return s
		}
	}
	return ""
}

// ServerIPV6 is the address the server takes in its own IPv6 subnet: the first
// host, so fd42:ab12:: becomes fd42:ab12::1. It returns "" for anything that
// is not an IPv6 address.
func ServerIPV6(network string) string {
	ip := net.ParseIP(network)
	if ip == nil || ip.To4() != nil {
		return ""
	}
	addr := ip.To16()
	addr[15] = 1
	return addr.String()
}

// NextFreeIPv6 returns the lowest host address in subnet that is not in used,
// or "" when the subnet is malformed or the scan gave up. Unlike IPv4 there is
// no broadcast address to skip, so only the network address is held back.
func NextFreeIPv6(subnet string, used map[string]bool) string {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil || ipNet.IP.To4() != nil {
		return ""
	}

	ip := cloneIP(ipNet.IP)
	for range maxScanV6 {
		incrementIP(ip)
		if !ipNet.Contains(ip) {
			return ""
		}
		if s := ip.String(); !used[s] {
			return s
		}
	}
	return ""
}

func cloneIP(ip net.IP) net.IP {
	clone := make(net.IP, len(ip))
	copy(clone, ip)
	return clone
}

func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func isBroadcast(ip net.IP, ipNet *net.IPNet) bool {
	mask := ipNet.Mask
	broadcast := make(net.IP, len(ip))
	for i := range ip {
		broadcast[i] = ipNet.IP[i] | ^mask[i]
	}
	return ip.Equal(broadcast)
}
