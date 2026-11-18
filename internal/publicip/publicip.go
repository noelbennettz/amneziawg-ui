// Package publicip finds the address clients should connect to, which a
// container behind NAT cannot read off its own interfaces.
package publicip

import (
	"context"
	"net/http"
	"strings"
	"time"

	"amneziawg-web-ui/internal/netutil"
)

// Unknown is what every generated config shows when nothing could tell us
// the address; the operator sees it and sets an endpoint by hand.
const Unknown = "YOUR_SERVER_IP"

// Services answer a plain GET with the caller's IPv4 address; Services6 with
// its IPv6 address. The v6 list is only asked when no v4 service answered,
// which costs a v6-only host a few seconds at startup and costs a dual-stack
// host nothing.
var (
	// Both lists are HTTPS. The address that comes back from them is written
	// into every client config as the Endpoint, so a plaintext answer is an
	// answer an on-path attacker can choose: every client the panel hands a
	// config to would then dial the attacker's server instead.
	Services  = []string{"https://api.ipify.org", "https://ident.me", "https://ifconfig.me"}
	Services6 = []string{"https://api6.ipify.org", "https://v6.ident.me", "https://ipv6.icanhazip.com"}

	// DetectTimeout bounds one whole list. Three services that each hang for
	// the full per-request timeout would otherwise cost 15 seconds per family
	// - and this runs before the panel starts listening.
	DetectTimeout = 6 * time.Second
)

// Detect returns the address clients should connect to: the first IPv4 answer,
// then the first IPv6 one, then whatever the host's own routing tables say -
// v4 before v6, and Unknown if nothing at all answered.
//
// IPv4 goes first on purpose. It is the address every client can still reach,
// so a dual-stack host does not start handing out an IPv6 endpoint that half
// the phones out there cannot use. The IPv6 pass exists for the hosts the old
// code had no answer for at all: a v6-only machine used to get
// YOUR_SERVER_IP, and every config generated from it was broken.
func Detect(fallbackV4, fallbackV6 func() (string, error)) string {
	if ip := detectFrom(Services, netutil.IsIPv4); ip != "" {
		return ip
	}
	if ip := detectFrom(Services6, netutil.IsIPv6); ip != "" {
		return ip
	}

	for _, fallback := range []struct {
		ask    func() (string, error)
		accept func(string) bool
	}{
		{fallbackV4, netutil.IsIPv4},
		{fallbackV6, netutil.IsIPv6},
	} {
		if fallback.ask == nil {
			continue
		}
		if ip, err := fallback.ask(); err == nil && fallback.accept(ip) {
			return ip
		}
	}
	return Unknown
}

// detectFrom asks one list of services in turn and returns the first answer
// accept approves of, so a service that answers with the wrong family - the
// way http://ifconfig.me does for an IPv6 caller - cannot be mistaken for the
// address of that family.
//
// The whole list shares one deadline: a host whose first two services are
// unreachable should not wait out a per-request timeout on each.
func detectFrom(services []string, accept func(string) bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), DetectTimeout)
	defer cancel()

	client := &http.Client{Timeout: 4 * time.Second}
	for _, svc := range services {
		if ctx.Err() != nil {
			return ""
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, svc, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		buf := make([]byte, 64)
		n, _ := resp.Body.Read(buf)
		resp.Body.Close()
		ip := strings.TrimSpace(string(buf[:n]))
		if accept(ip) {
			return ip
		}
	}
	return ""
}
