package wgconf

import (
	"fmt"
	"strings"
	"time"

	"amneziawg-web-ui/web-ui/api"
)

// DefaultAllowedIPs routes everything through the tunnel, both stacks: the
// client's own IPv6 address is pointless if ::/0 is not also sent through it.
const DefaultAllowedIPs = "0.0.0.0/0, ::/0"

// AddressList joins the addresses one Address line carries. WireGuard reads a
// comma separated list, and one line is what every implementation of the
// format - wg-quick, the AmneziaVPN importer, the mobile apps - handles the
// same way. Empty entries are dropped, so an IPv4-only config keeps the single
// address it always had.
func AddressList(addresses ...string) string {
	present := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if trimmed := strings.TrimSpace(address); trimmed != "" {
			present = append(present, trimmed)
		}
	}
	return strings.Join(present, ", ")
}

// PeerAllowedIPs is what the server routes to one client: the client's own
// addresses on both stacks, whatever the client itself sends through the
// tunnel. Without the IPv6 entry the server would drop every packet addressed
// to the client's IPv6 address, which the client routes into the tunnel by
// default.
func PeerAllowedIPs(client *api.Client) string {
	allowed := []string{client.ClientIP + "/32"}
	if client.ClientIPV6 != "" {
		allowed = append(allowed, client.ClientIPV6+"/128")
	}
	return AddressList(allowed...)
}

// DefaultPersistentKeepalive keeps a NAT mapping alive from the client side.
const DefaultPersistentKeepalive = "25"

// ServerInterface is what a new server's .conf starts as: the [Interface]
// section alone, peers get appended as clients are added.
type ServerInterface struct {
	PrivateKey string
	Address    string // "10.0.0.1/24"
	// AddressV6 is the server's address in its IPv6 subnet ("fd42:ab12::1/64"),
	// written on the same Address line. Empty writes IPv4 only, which is what
	// a server without an IPv6 subnet gets.
	AddressV6  string
	ListenPort int
	MTU        int

	// Obfuscation is nil when the server runs plain WireGuard.
	Obfuscation *api.ObfuscationParams
}

// Render produces the [Interface] section.
func (s ServerInterface) Render() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[Interface]\n")
	fmt.Fprintf(&sb, "PrivateKey = %s\n", s.PrivateKey)
	fmt.Fprintf(&sb, "Address = %s\n", AddressList(s.Address, s.AddressV6))
	fmt.Fprintf(&sb, "ListenPort = %d\n", s.ListenPort)
	fmt.Fprintf(&sb, "SaveConfig = false\n")
	fmt.Fprintf(&sb, "MTU = %d\n", s.MTU)
	writeObfuscation(&sb, s.Obfuscation)
	return sb.String()
}

// ClientConf renders the .conf a client imports. endpoint is the host the
// client connects to; the caller resolves it, since the fallback chain ends
// at the backend's detected public IP.
func ClientConf(srv *api.Server, client *api.Client, endpoint string, includeComments bool) string {
	var sb strings.Builder

	if includeComments {
		fmt.Fprintf(&sb, "# AmneziaWG Client Configuration\n")
		fmt.Fprintf(&sb, "# Server: %s\n", srv.Name)
		fmt.Fprintf(&sb, "# Client: %s\n", client.Name)
		fmt.Fprintf(&sb, "# Generated: %s\n", time.Unix(int64(client.CreatedAt), 0).UTC().String())
		fmt.Fprintf(&sb, "# Server Endpoint: %s:%d\n", endpoint, srv.Port)
	}

	fmt.Fprintf(&sb, "[Interface]\n")
	fmt.Fprintf(&sb, "PrivateKey = %s\n", client.ClientPrivateKey)
	fmt.Fprintf(&sb, "Address = %s\n", AddressList(client.ClientIP+"/32", clientAddressV6(client)))
	fmt.Fprintf(&sb, "DNS = %s\n", strings.Join(srv.DNS, ", "))
	fmt.Fprintf(&sb, "MTU = %d\n", srv.MTU)

	if client.ObfuscationEnabled {
		writeObfuscation(&sb, client.ObfuscationParams)
	}

	// I1 is the signature packet the rest hang off: without it the others
	// are not sent at all, so they are not written either.
	if client.ApplyISettings && client.ISettings["i1"] != "" {
		for n := 1; n <= 5; n++ {
			if v := client.ISettings[fmt.Sprintf("i%d", n)]; v != "" {
				fmt.Fprintf(&sb, "I%d = %s\n", n, v)
			}
		}
	}

	fmt.Fprintf(&sb, "\n[Peer]\n")
	fmt.Fprintf(&sb, "PublicKey = %s\n", srv.ServerPublicKey)
	fmt.Fprintf(&sb, "PresharedKey = %s\n", client.PresharedKey)
	fmt.Fprintf(&sb, "Endpoint = %s:%d\n", endpoint, srv.Port)
	fmt.Fprintf(&sb, "AllowedIPs = %s\n", AllowedIPsOrDefault(client.AllowedIPs))
	fmt.Fprintf(&sb, "PersistentKeepalive = %s\n", PersistentKeepalive(client.ObfuscationParams))

	return sb.String()
}

// clientAddressV6 is the client's IPv6 address as an Address line wants it: a
// single host, so /128. Empty for a client without an IPv6 address, which
// leaves the line IPv4-only.
func clientAddressV6(client *api.Client) string {
	if client.ClientIPV6 == "" {
		return ""
	}
	return client.ClientIPV6 + "/128"
}

// AllowedIPsOrDefault substitutes the full-tunnel default for a blank value.
func AllowedIPsOrDefault(allowedIPs string) string {
	if strings.TrimSpace(allowedIPs) == "" {
		return DefaultAllowedIPs
	}
	return strings.TrimSpace(allowedIPs)
}

// PersistentKeepalive is the client's keepalive, from its obfuscation
// parameters when set there.
func PersistentKeepalive(p *api.ObfuscationParams) string {
	if p != nil && p.PersistentKeepalive != "" {
		return p.PersistentKeepalive
	}
	return DefaultPersistentKeepalive
}

// writeObfuscation writes the AmneziaWG parameter set, the same lines for the
// server's [Interface] and the client's: both ends must agree on every one.
// A nil set writes nothing, which is plain WireGuard.
func writeObfuscation(sb *strings.Builder, p *api.ObfuscationParams) {
	if p == nil {
		return
	}
	fmt.Fprintf(sb, "Jc = %d\n", p.Jc)
	fmt.Fprintf(sb, "Jmin = %d\n", p.Jmin)
	fmt.Fprintf(sb, "Jmax = %d\n", p.Jmax)
	fmt.Fprintf(sb, "S1 = %d\n", p.S1)
	fmt.Fprintf(sb, "S2 = %d\n", p.S2)
	fmt.Fprintf(sb, "S3 = %d\n", p.S3)
	fmt.Fprintf(sb, "S4 = %d\n", p.S4)
	fmt.Fprintf(sb, "H1 = %d\n", p.H1)
	fmt.Fprintf(sb, "H2 = %d\n", p.H2)
	fmt.Fprintf(sb, "H3 = %d\n", p.H3)
	fmt.Fprintf(sb, "H4 = %d\n", p.H4)
	writeIfSet(sb, "HeaderProtectionKey", p.HeaderProtectionKey)
	writeIfSet(sb, "ContentPaddingAddition", p.ContentPaddingAddition)
	writeIfSet(sb, "RekeyAfterTime", p.RekeyAfterTime)
	writeIfSet(sb, "RekeyTimeout", p.RekeyTimeout)
	writeIfSet(sb, "RejectAfterTime", p.RejectAfterTime)
	writeIfSet(sb, "KeepaliveTimeout", p.KeepaliveTimeout)
	writeIfSet(sb, "MaxHandshakeAttempts", p.MaxHandshakeAttempts)
	writeAwgBoolIfOn(sb, "RandomTrailers", p.RandomTrailers)
	writeAwgBoolIfOn(sb, "DisableCookies", p.DisableCookies)
}

// writeIfSet writes "key = value\n" to sb only if value is non-empty.
func writeIfSet(sb *strings.Builder, key, value string) {
	if value != "" {
		fmt.Fprintf(sb, "%s = %s\n", key, value)
	}
}

// writeAwgBoolIfOn writes an AmneziaWG 3.1 "key = on" line, and writes
// nothing when the flag is off. amneziawg-tools accepts on/off and 0/1, but
// an omitted key is the only form every pre-3.1 parser tolerates - clients
// still on AmneziaWG 3.0 reject an unknown key outright and would fail to
// import the config, so "off" is expressed by silence.
func writeAwgBoolIfOn(sb *strings.Builder, key string, enabled bool) {
	if enabled {
		fmt.Fprintf(sb, "%s = on\n", key)
	}
}
