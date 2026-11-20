package awg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Tools wraps the host commands behind names that say what they are for.
// Nothing here knows about servers or clients; it takes interface names and
// subnets and reports what the host said.
type Tools struct {
	run Runner

	// ScriptsDir holds setup_iptables.sh and cleanup_iptables.sh. A
	// deployment without them - a development tree - gets an error from the
	// firewall step rather than a silent skip: the rules the caller believes
	// it applied are not there either way.
	ScriptsDir string

	// BinDir is where awg and awg-quick are installed.
	BinDir string
}

// New returns Tools issuing commands through run, with the container's
// default locations.
func New(run Runner) *Tools {
	return &Tools{run: run, ScriptsDir: "/app/scripts", BinDir: "/usr/bin"}
}

// Available reports whether both amneziawg-tools binaries are installed.
func (t *Tools) Available() bool {
	_, awgErr := os.Stat(filepath.Join(t.BinDir, "awg"))
	_, quickErr := os.Stat(filepath.Join(t.BinDir, "awg-quick"))
	return awgErr == nil && quickErr == nil
}

// QuickUp brings an interface up from its .conf.
func (t *Tools) QuickUp(iface string) error {
	if _, err := t.run.Run(fmt.Sprintf("%s up %s", filepath.Join(t.BinDir, "awg-quick"), iface)); err != nil {
		return fmt.Errorf("awg-quick up %s: %w", iface, quickFailure(err))
	}
	return nil
}

// QuickDown tears an interface down.
func (t *Tools) QuickDown(iface string) error {
	if _, err := t.run.Run(fmt.Sprintf("%s down %s", filepath.Join(t.BinDir, "awg-quick"), iface)); err != nil {
		return fmt.Errorf("awg-quick down %s: %w", iface, quickFailure(err))
	}
	return nil
}

// quickFailure cuts awg-quick's stderr down to the output of the command that
// failed. awg-quick echoes each step as "[#] command", and a start that falls
// back to userspace also carries the kernel attempt's "Unknown device type",
// a "[!]" notice and a box-drawn banner - none of it the reason, all of it
// ahead of the one line that is. The last step with output is that line: the
// steps after a failure are awg-quick's own cleanup, and they say nothing.
func quickFailure(err error) error {
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		return err
	}
	var last, current []string
	inBanner := false
	for line := range strings.SplitSeq(exitErr.Stderr, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "┌"):
			inBanner = true
		case strings.HasPrefix(line, "└"):
			inBanner = false
		case inBanner, line == "", strings.HasPrefix(line, "[!]"):
		case strings.HasPrefix(line, "[#]"):
			current = nil
		default:
			current = append(current, line)
			last = current
		}
	}
	if len(last) == 0 {
		return err
	}
	return &ExitError{Err: exitErr.Err, Stderr: strings.Join(last, "\n")}
}

// SyncConf pushes the interface's .conf onto the running interface without
// taking it down, so existing peers keep their sessions.
func (t *Tools) SyncConf(iface string) error {
	cmd := fmt.Sprintf("bash -c 'awg syncconf %s <(awg-quick strip %s)'", iface, iface)
	_, err := t.run.Run(cmd)
	return err
}

// InterfaceUp reports whether the kernel has the interface. It shells out,
// so callers must not hold a lock other requests wait on while asking.
func (t *Tools) InterfaceUp(iface string) bool {
	if iface == "" {
		return false
	}
	result, err := t.run.Run(fmt.Sprintf("ip link show %s", iface))
	if err != nil {
		return false
	}
	return strings.Contains(result, "state UNKNOWN") || strings.Contains(result, iface)
}

// NATStatus is what the firewall script decided about one address family:
// the state of the way out, the interface the routing table named, and the
// address a masquerade would rewrite the source to.
type NATStatus struct {
	// State is one of "ok", "ula-only", "no-route", "no-address",
	// "unavailable" or "failed". Empty means the script could not be asked.
	State  string
	Uplink string
	Source string
}

// FirewallStatus is the per-family outcome of SetupIPTables and NATState.
type FirewallStatus struct {
	V4, V6 NATStatus
}

// ParseNATStatus reads the "NAT4 state=ok uplink=eth0 source=203.0.113.9"
// lines the firewall script prints. Every other line is ignored, so the
// script's human-readable output can change without breaking this.
func ParseNATStatus(output string) FirewallStatus {
	var status FirewallStatus
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		var family *NATStatus
		switch fields[0] {
		case "NAT4":
			family = &status.V4
		case "NAT6":
			family = &status.V6
		default:
			continue
		}
		for _, field := range fields[1:] {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			switch key {
			case "state":
				family.State = value
			case "uplink":
				family.Uplink = value
			case "source":
				family.Source = value
			}
		}
	}
	return status
}

// SetupIPTables adds the forwarding and NAT rules for an interface, if the
// script for it is deployed. subnetV6 is the server's ULA /64; the script adds
// the matching ip6tables rules - the NAT66 masquerade included - only when it
// is not empty.
//
// The returned status says what the script decided about each family. A
// family with no default route, or with an uplink that carries only a unique
// local address, gets no NAT rule (or one that cannot reach the internet);
// that is the difference between "connected, no internet" and a broken
// server, and it is reported rather than left in a log line nobody reads.
//
// The error is the caller's to report: a server whose port and addresses are
// wrong announces it by not working, but one that is up with no NAT accepts
// every handshake and forwards nothing, which looks like a dead uplink from
// the client's side.
func (t *Tools) SetupIPTables(iface, subnet, subnetV6 string) (FirewallStatus, error) {
	out, err := t.runScript("setup_iptables.sh", iface, subnet, subnetV6)
	return ParseNATStatus(out), err
}

// CleanupIPTables removes what SetupIPTables added.
func (t *Tools) CleanupIPTables(iface, subnet, subnetV6 string) error {
	_, err := t.runScript("cleanup_iptables.sh", iface, subnet, subnetV6)
	return err
}

// NATState asks the firewall script what it would decide now, without
// touching a rule. An empty FirewallStatus means the script could not be
// asked - it is not deployed, or it failed.
func (t *Tools) NATState() FirewallStatus {
	script := filepath.Join(t.ScriptsDir, "setup_iptables.sh")
	if _, err := os.Stat(script); err != nil {
		return FirewallStatus{}
	}
	out, err := t.run.Run(script + " --status")
	if err != nil {
		return FirewallStatus{}
	}
	return ParseNATStatus(out)
}

// runScript runs one of the two iptables scripts and returns its output. A
// script that is not deployed and a script that fails are both errors: either
// way the rules the caller believes it applied are not there.
func (t *Tools) runScript(name, iface, subnet, subnetV6 string) (string, error) {
	script := filepath.Join(t.ScriptsDir, name)
	if _, err := os.Stat(script); err != nil {
		return "", fmt.Errorf("%s is not deployed at %s: %w", name, script, err)
	}
	out, err := t.run.Run(fmt.Sprintf("%s %s %s %s", script, iface, subnet, subnetV6))
	if err != nil {
		return out, fmt.Errorf("%s %s: %w", name, iface, err)
	}
	return out, nil
}

// CheckIPTables looks for the rules SetupIPTables should have left, keyed by
// the command that looked. The values are "Found" or "Not found". The ip6tables
// checks only appear for a server with an IPv6 subnet, since those are the only
// servers the script writes them for.
//
// The rules are read with -S, not -L: the plain listing does not print the
// interface a rule is scoped to, so grepping it for the tunnel's name never
// matched a rule that was there. The name is matched with the space that
// follows it, so wg-1 does not answer for wg-10. The IPv6 address check wants
// 2000::/3 as well - "scope global" is true of the unique local address a
// Docker bridge hands out, which is exactly the address that cannot reach the
// internet.
//
// The script's own per-family verdict comes back alongside the map: a rule
// can be present and still have nothing to masquerade onto, and the state is
// what tells those apart.
func (t *Tools) CheckIPTables(iface, subnet, subnetV6 string) (map[string]string, FirewallStatus) {
	checks := []string{
		fmt.Sprintf("iptables -S INPUT | grep -F -- '-i %s '", iface),
		fmt.Sprintf("iptables -S FORWARD | grep -F -- '-i %s '", iface),
		fmt.Sprintf("iptables -t nat -S POSTROUTING | grep -F -- '%s'", subnet),
		// Without a default route the masquerade above matches nothing, which
		// is a rule that reads "Found" and does not work.
		"ip route show default | head -1",
	}
	if subnetV6 != "" {
		checks = append(checks,
			fmt.Sprintf("ip6tables -S INPUT | grep -F -- '-i %s '", iface),
			fmt.Sprintf("ip6tables -S FORWARD | grep -F -- '-i %s '", iface),
			fmt.Sprintf("ip6tables -t nat -S POSTROUTING | grep -F -- '%s'", subnetV6),
			"ip -6 route show default | head -1",
			"ip -6 -o addr show scope global | grep -E 'inet6 [23][0-9a-fA-F]{3}:' | head -1",
		)
	}
	results := map[string]string{}
	for _, cmd := range checks {
		out, err := t.run.Run(cmd)
		if err == nil && out != "" {
			results[cmd] = "Found"
		} else {
			results[cmd] = "Not found"
		}
	}
	return results, t.NATState()
}

// RouteSourceIP is the address the host would use to reach the internet,
// from its routing table. It is the fallback when no external service could
// say what the public address is.
//
// The field is looked up by name, like RouteSourceIP6: $7 is the source only
// when the default route goes through a gateway. A `default dev ppp0` line
// prints "dev ... src <ip> uid <n>", where the seventh word is the uid - a
// number that used to be handed back as if it were an address.
func (t *Tools) RouteSourceIP() (string, error) {
	return t.run.Run("ip route get 1 | awk '{for (i = 1; i < NF; i++) if ($i == \"src\") { print $(i + 1); exit }}'")
}

// RouteSourceIP6 is RouteSourceIP for IPv6. The field is looked up by name
// rather than by position: an IPv6 route prints its own "from ::" before the
// gateway, so the source address is not the seventh word the way it is for
// IPv4.
func (t *Tools) RouteSourceIP6() (string, error) {
	return t.run.Run("ip -6 route get 2001:4860:4860::8888 2>/dev/null" +
		" | awk '{for (i = 1; i < NF; i++) if ($i == \"src\") { print $(i + 1); exit }}'")
}

// PeerStats is what `awg show` reports about one peer: the exact byte counts
// of the `transfer` listing, the endpoint and handshake age as the plain
// listing words them, and the moment of that handshake from the
// `latest-handshakes` listing (zero when the peer never shook hands).
type PeerStats struct {
	RXBytes, TXBytes        uint64
	LastHandshake, Endpoint string
	HandshakeAt             time.Time
}

// ShowPeers reads the peers of an interface, keyed by public key.
//
// It asks for the dump format first: one call to awg that carries every figure
// this needs, instead of three calls that each make the kernel dump the whole
// peer table again. The panel polls this every few seconds for every open tab,
// so the difference is three processes per server per poll against one. Older
// amneziawg-tools without `dump` fall back to the three listings, unchanged.
func (t *Tools) ShowPeers(iface string) map[string]PeerStats {
	awg := filepath.Join(t.BinDir, "awg")

	if dump, err := t.run.Run(fmt.Sprintf("%s show %s dump", awg, iface)); err == nil {
		return ParsePeerDump(dump, time.Now())
	}

	output, err := t.run.Run(fmt.Sprintf("%s show %s", awg, iface))
	if err != nil || output == "" {
		return nil
	}
	peers := ParseShow(output)
	transfer, _ := t.run.Run(fmt.Sprintf("%s show %s transfer", awg, iface))
	for key, tr := range ParseTransfer(transfer) {
		p := peers[key]
		p.RXBytes, p.TXBytes = tr.RXBytes, tr.TXBytes
		peers[key] = p
	}
	handshakes, _ := t.run.Run(fmt.Sprintf("%s show %s latest-handshakes", awg, iface))
	for key, at := range ParseLatestHandshakes(handshakes) {
		if p, ok := peers[key]; ok {
			p.HandshakeAt = at
			peers[key] = p
		}
	}
	return peers
}

// ParsePeerDump reads `awg show <iface> dump`, whose peer lines are
//
//	<pubkey> <psk> <endpoint> <allowed ips> <handshake> <rx> <tx> <keepalive>
//
// tab separated, with "(none)" for an endpoint or a handshake there is not.
// The handshake is a unix timestamp here rather than the wording of the plain
// listing, so its age is worded the way awg words it.
func ParsePeerDump(output string, now time.Time) map[string]PeerStats {
	peers := map[string]PeerStats{}
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		rx, errRX := strconv.ParseUint(strings.TrimSpace(fields[5]), 10, 64)
		tx, errTX := strconv.ParseUint(strings.TrimSpace(fields[6]), 10, 64)
		if errRX != nil || errTX != nil {
			continue
		}
		stats := PeerStats{RXBytes: rx, TXBytes: tx, LastHandshake: "Never"}
		if endpoint := strings.TrimSpace(fields[2]); endpoint != "(none)" {
			stats.Endpoint = endpoint
		}
		if sec, err := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64); err == nil && sec > 0 {
			stats.HandshakeAt = time.Unix(sec, 0)
			stats.LastHandshake = FormatHandshakeAge(now.Sub(stats.HandshakeAt))
		}
		peers[strings.TrimSpace(fields[0])] = stats
	}
	return peers
}

// FormatHandshakeAge words an age the way `awg show` does: every non-zero unit
// from years down, comma separated, always ending in seconds and " ago".
func FormatHandshakeAge(age time.Duration) string {
	seconds := max(int64(age/time.Second), 0)

	units := []struct {
		name    string
		seconds int64
	}{
		{"year", 365 * 24 * 60 * 60},
		{"day", 24 * 60 * 60},
		{"hour", 60 * 60},
		{"minute", 60},
	}

	var parts []string
	for _, unit := range units {
		if seconds < unit.seconds {
			continue
		}
		n := seconds / unit.seconds
		seconds %= unit.seconds
		parts = append(parts, plural(n, unit.name))
	}
	parts = append(parts, plural(seconds, "second"))

	return strings.Join(parts, ", ") + " ago"
}

func plural(n int64, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.FormatInt(n, 10) + " " + unit + "s"
}

// ParseShow reads the output of `awg show`: who the peers are, where they
// are, and when they last shook hands. It is separate from ShowPeers so the
// parser can be tested on captured output.
func ParseShow(output string) map[string]PeerStats {
	peers := map[string]PeerStats{}
	current := ""
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "peer:"):
			current = strings.TrimSpace(strings.TrimPrefix(line, "peer:"))
			peers[current] = PeerStats{LastHandshake: "Never"}
		case current == "":
			continue
		case strings.HasPrefix(line, "endpoint:"):
			p := peers[current]
			p.Endpoint = strings.TrimSpace(strings.TrimPrefix(line, "endpoint:"))
			peers[current] = p
		case strings.HasPrefix(line, "latest handshake:"):
			p := peers[current]
			p.LastHandshake = strings.TrimSpace(strings.TrimPrefix(line, "latest handshake:"))
			peers[current] = p
		}
	}
	return peers
}

// Transfer is one line of `awg show <iface> transfer`: what the interface
// received from a peer and sent to it, in bytes.
type Transfer struct {
	RXBytes, TXBytes uint64
}

// ParseTransfer reads `awg show <iface> transfer`, one "<key>\t<rx>\t<tx>"
// line per peer. Lines that do not fit are skipped.
func ParseTransfer(output string) map[string]Transfer {
	peers := map[string]Transfer{}
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		rx, errRX := strconv.ParseUint(fields[1], 10, 64)
		tx, errTX := strconv.ParseUint(fields[2], 10, 64)
		if errRX != nil || errTX != nil {
			continue
		}
		peers[fields[0]] = Transfer{RXBytes: rx, TXBytes: tx}
	}
	return peers
}

// ParseLatestHandshakes reads `awg show <iface> latest-handshakes`, one
// "<key>\t<unix seconds>" line per peer. A zero timestamp is a peer that
// never shook hands and is left out, as are lines that do not fit.
func ParseLatestHandshakes(output string) map[string]time.Time {
	peers := map[string]time.Time{}
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		sec, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || sec <= 0 {
			continue
		}
		peers[fields[0]] = time.Unix(sec, 0)
	}
	return peers
}
