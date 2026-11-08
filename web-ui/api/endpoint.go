package api

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// MaxHostnameLength is the longest DNS name a client can be pointed at, in
// presentation form: 253 characters, the wire limit for a name whose labels
// are each at most 63.
const MaxHostnameLength = 253

// ParseEndpoint reads the host part of the address clients are told to
// connect to: an IPv4 literal, an IPv6 literal - with or without the brackets
// a .conf file writes it in, since both are what an operator copies from
// somewhere - or a DNS name.
//
// What comes back is the host on its own: an IPv6 literal without brackets
// (EndpointAddress puts them back where the config format needs them) and a
// name as it was typed. A port is refused rather than stripped - the port is
// the server's own and is appended to this field later, so "vpn.example.com:
// 51820" here can only mean the field was misread, and accepting it would
// produce an Endpoint line carrying the port twice.
//
// An empty string is not an endpoint: callers decide what an empty field
// means (the backend substitutes the detected public address).
func ParseEndpoint(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "", fmt.Errorf("endpoint is empty")
	}
	if strings.Contains(host, "://") {
		return "", fmt.Errorf("endpoint %q is a URL: give the host name or IP address alone, without a scheme", raw)
	}

	// A bracketed literal is the form a .conf file uses; the brackets belong
	// to that format, not to the address, so they are dropped here and added
	// back by EndpointAddress.
	if strings.HasPrefix(host, "[") {
		if !strings.HasSuffix(host, "]") {
			return "", fmt.Errorf("endpoint %q opens a bracket it never closes", raw)
		}
		return ipv6Host(raw, host[1:len(host)-1])
	}

	// Only an address can carry a colon; a name cannot, and neither can a
	// path. Both mean the field holds something other than a host.
	if strings.ContainsAny(host, ":/") {
		return ipv6Host(raw, host)
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.String(), nil
	}
	if err := validateHostname(host); err != nil {
		return "", fmt.Errorf("endpoint %q is not a host name or an IP address: %w", raw, err)
	}
	return host, nil
}

// ipv6Host is the shared tail of the two colon-carrying cases: the field has
// to be an IPv6 literal, and anything else is reported as the mistake it is.
func ipv6Host(raw, host string) (string, error) {
	addr, err := netip.ParseAddr(host)
	if err != nil || !addr.Is6() {
		return "", fmt.Errorf("endpoint %q is not an IPv6 address: leave the port out (the server appends its own) and bracket an IPv6 literal as [2001:db8::1]", raw)
	}
	return addr.String(), nil
}

// validateHostname checks a DNS name the way a resolver would read it. It is
// deliberately narrow - letters, digits, hyphens and dots - because a name
// that only some resolver accepts is one that fails on somebody's phone.
func validateHostname(host string) error {
	if len(host) > MaxHostnameLength {
		return fmt.Errorf("it is longer than %d characters", MaxHostnameLength)
	}
	trimmed := strings.TrimSuffix(host, ".")
	if trimmed == "" {
		return fmt.Errorf("it has no labels")
	}
	for _, label := range strings.Split(trimmed, ".") {
		switch {
		case label == "":
			return fmt.Errorf("it has an empty label")
		case len(label) > 63:
			return fmt.Errorf("the label %q is longer than 63 characters", label)
		case strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-"):
			return fmt.Errorf("the label %q starts or ends with a hyphen", label)
		}
		for _, r := range label {
			letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
			digit := r >= '0' && r <= '9'
			if !letter && !digit && r != '-' {
				return fmt.Errorf("the label %q holds %q, which a host name cannot have", label, r)
			}
		}
	}
	return nil
}

// EndpointAddress is the value of an Endpoint line: the host and the port
// joined the way the WireGuard config format writes them. net.JoinHostPort
// brackets anything carrying a colon - which is what stops
// "2001:db8::1:51820" from reading as one long address - and brackets already
// in the field are dropped first, so a value copied out of another .conf does
// not end up doubled.
func EndpointAddress(host string, port int) string {
	return net.JoinHostPort(strings.TrimSpace(strings.Trim(host, "[]")), strconv.Itoa(port))
}

// IsIPAddress reports whether s is an IPv4 or IPv6 literal. The DNS field
// takes addresses only: a resolver named by a host name could not be resolved
// before the tunnel that needs it is up.
func IsIPAddress(s string) bool {
	_, err := netip.ParseAddr(strings.TrimSpace(s))
	return err == nil
}
