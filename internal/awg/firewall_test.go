package awg_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"amneziawg-web-ui/internal/awg"
	"amneziawg-web-ui/internal/awg/awgtest"
)

// The script's two lines are the contract between the shell and the panel:
// parseNATStatus is what the API and the log repeat, so the spelling has to
// survive any human-readable line that lands next to it.
func TestParseNATStatus(t *testing.T) {
	out := "Setting up iptables for interface wg-abc123 with subnet 10.0.1.0/24\n" +
		"NAT4 state=ok uplink=eth0 source=203.0.113.9\n" +
		"NAT6 state=ula-only uplink=eth0 source=fd00:dead:beef::2\n" +
		"iptables rules set up successfully for wg-abc123\n"

	status := awg.ParseNATStatus(out)
	if status.V4.State != "ok" || status.V4.Uplink != "eth0" || status.V4.Source != "203.0.113.9" {
		t.Errorf("V4 = %+v", status.V4)
	}
	if status.V6.State != "ula-only" || status.V6.Uplink != "eth0" || status.V6.Source != "fd00:dead:beef::2" {
		t.Errorf("V6 = %+v", status.V6)
	}

	empty := awg.ParseNATStatus("no state lines here\n")
	if empty.V4.State != "" || empty.V6.State != "" {
		t.Errorf("output without state lines = %+v, want empty", empty)
	}
}

// SetupIPTables used to answer with an error and nothing else, so a family
// with no route and a family with a working masquerade looked the same to the
// caller. The script's decision is now carried back with it.
func TestSetupIPTablesReturnsTheScriptsDecision(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "setup_iptables.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := awgtest.New().Stub(script,
		"NAT4 state=ok uplink=eth0 source=203.0.113.9\n"+
			"NAT6 state=no-route\n")
	tools := awg.New(run)
	tools.ScriptsDir = dir

	status, err := tools.SetupIPTables("wg-abc123", "10.0.1.0/24", "fd42:ab12::/64")
	if err != nil {
		t.Fatalf("SetupIPTables = %v", err)
	}
	if status.V4.State != "ok" {
		t.Errorf("V4 = %+v, want ok", status.V4)
	}
	if status.V6.State != "no-route" || status.V6.Uplink != "" {
		t.Errorf("V6 = %+v, want no-route with no uplink", status.V6)
	}
}

// The rule check is read off the rule set, not off the plain listing: -L
// prints no interface name, so grepping it for the tunnel's name never
// matched a rule that was there. The IPv6 address check wants 2000::/3, since
// a unique local address is "scope global" too and is exactly the address a
// Docker bridge hands out.
func TestCheckIPTablesAsksForTheRuleSet(t *testing.T) {
	tools := awg.New(awgtest.New())
	tools.ScriptsDir = t.TempDir() // no script: the verdict is unknown

	check, status := tools.CheckIPTables("wg-abc123", "10.0.1.0/24", "fd42:ab12::/64")

	asked := []string{
		"iptables -S INPUT | grep -F -- '-i wg-abc123 '",
		"iptables -S FORWARD | grep -F -- '-i wg-abc123 '",
		"iptables -t nat -S POSTROUTING | grep -F -- '10.0.1.0/24'",
		"ip route show default | head -1",
		"ip6tables -S INPUT | grep -F -- '-i wg-abc123 '",
		"ip6tables -S FORWARD | grep -F -- '-i wg-abc123 '",
		"ip6tables -t nat -S POSTROUTING | grep -F -- 'fd42:ab12::/64'",
		"ip -6 route show default | head -1",
		"ip -6 -o addr show scope global | grep -E 'inet6 [23][0-9a-fA-F]{3}:' | head -1",
	}
	for _, want := range asked {
		if _, ok := check[want]; !ok {
			t.Errorf("the check never ran %q: %v", want, check)
		}
	}
	for cmd := range check {
		if strings.Contains(cmd, " -L ") {
			t.Errorf("the check still reads a plain listing: %q", cmd)
		}
	}
	if status.V4.State != "" || status.V6.State != "" {
		t.Errorf("status without a script = %+v, want empty", status)
	}
}

// The script is run for real here, against fake ip/iptables binaries: what a
// family with a route gets, and what one without it must not get, is decided
// in shell, and a fake Runner would only prove that the command was issued.
func TestSetupScriptWritesNATOnlyForAFamilyWithARoute(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "setup_iptables.sh"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name         string
		route4       string
		route6       string
		addr4        string
		addr6        string
		want4, want6 string
		wantNAT4     bool
		wantNAT6     bool
	}{
		{
			name:     "dual stack",
			route4:   "default via 203.0.113.1 dev eth0 proto dhcp src 203.0.113.9 metric 100",
			route6:   "default via fe80::1 dev eth0 proto ra metric 100",
			addr4:    "2: eth0    inet 203.0.113.9/24 scope global eth0",
			addr6:    "2: eth0    inet6 2001:db8:1::2/64 scope global dynamic eth0",
			want4:    "ok",
			want6:    "ok",
			wantNAT4: true,
			wantNAT6: true,
		},
		{
			name:     "ipv4 only host",
			route4:   "default via 203.0.113.1 dev eth0 proto dhcp src 203.0.113.9 metric 100",
			addr4:    "2: eth0    inet 203.0.113.9/24 scope global eth0",
			want4:    "ok",
			want6:    "no-route",
			wantNAT4: true,
		},
		{
			name:     "ipv6 only host",
			route6:   "default via fe80::1 dev eth0 proto ra metric 100",
			addr6:    "2: eth0    inet6 2001:db8:1::2/64 scope global dynamic eth0",
			want4:    "no-route",
			want6:    "ok",
			wantNAT6: true,
		},
		{
			name:     "docker bridge with a unique local address",
			route4:   "default via 172.17.0.1 dev eth0",
			route6:   "default via fddd:7702:365d::1 dev eth0 metric 1024",
			addr4:    "2: eth0    inet 172.17.0.2/16 scope global eth0",
			addr6:    "2: eth0    inet6 fddd:7702:365d::2/64 scope global eth0",
			want4:    "ok",
			want6:    "ula-only",
			wantNAT4: true,
			wantNAT6: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			log := filepath.Join(bin, "iptables.log")
			writeStub(t, bin, "ip", `#!/bin/sh
case "$*" in
  "route show default") printf '%s\n' "$ROUTE4" ;;
  "-6 route show default") printf '%s\n' "$ROUTE6" ;;
  "-4 -o addr show dev "*) printf '%s\n' "$ADDR4" ;;
  "-6 -o addr show dev "*) printf '%s\n' "$ADDR6" ;;
esac
`)
			for _, tool := range []string{"iptables", "ip6tables"} {
				writeStub(t, bin, tool, `#!/bin/sh
echo "`+tool+` $*" >> "$IPT_LOG"
`)
			}

			cmd := exec.Command(bash, script, "wg-test", "10.0.1.0/24", "fd42:ab12::/64")
			cmd.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"),
				"IPT_LOG="+log,
				"ROUTE4="+tc.route4, "ROUTE6="+tc.route6,
				"ADDR4="+tc.addr4, "ADDR6="+tc.addr6)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("the script failed: %v\n%s", err, out)
			}

			status := awg.ParseNATStatus(string(out))
			if status.V4.State != tc.want4 {
				t.Errorf("IPv4 state = %q, want %q\n%s", status.V4.State, tc.want4, out)
			}
			if status.V6.State != tc.want6 {
				t.Errorf("IPv6 state = %q, want %q\n%s", status.V6.State, tc.want6, out)
			}

			rules, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(rules), "iptables -t nat -A POSTROUTING -s 10.0.1.0/24"); got != tc.wantNAT4 {
				t.Errorf("IPv4 masquerade written = %v, want %v\n%s", got, tc.wantNAT4, rules)
			}
			if got := strings.Contains(string(rules), "ip6tables -t nat -A POSTROUTING -s fd42:ab12::/64"); got != tc.wantNAT6 {
				t.Errorf("IPv6 masquerade written = %v, want %v\n%s", got, tc.wantNAT6, rules)
			}
		})
	}
}

// --status is what the panel asks when it only wants to know: it must not add
// a single rule.
func TestSetupScriptStatusChangesNothing(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "setup_iptables.sh"))
	if err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	log := filepath.Join(bin, "iptables.log")
	writeStub(t, bin, "ip", `#!/bin/sh
case "$*" in
  "route show default") echo "default via 203.0.113.1 dev eth0" ;;
  "-4 -o addr show dev "*) echo "2: eth0    inet 203.0.113.9/24 scope global eth0" ;;
esac
`)
	for _, tool := range []string{"iptables", "ip6tables"} {
		writeStub(t, bin, tool, "#!/bin/sh\necho \"$*\" >> \"$IPT_LOG\"\n")
	}

	cmd := exec.Command(bash, script, "--status")
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "IPT_LOG="+log)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--status failed: %v\n%s", err, out)
	}
	if status := awg.ParseNATStatus(string(out)); status.V4.State != "ok" {
		t.Errorf("--status = %q\n%s", status.V4.State, out)
	}
	if _, err := os.Stat(log); err == nil {
		t.Errorf("--status ran iptables: %s", log)
	}
}

func writeStub(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// --status is the read-only question the check asks, so the panel can repeat
// the script's verdict without writing a rule to get it.
func TestNATStateAsksTheScriptWithoutWritingAnything(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "setup_iptables.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := awgtest.New().Stub(script+" --status",
		"NAT4 state=ok uplink=eth0 source=203.0.113.9\nNAT6 state=no-route\n")
	tools := awg.New(run)
	tools.ScriptsDir = dir

	status := tools.NATState()
	if status.V4.State != "ok" || status.V4.Source != "203.0.113.9" {
		t.Errorf("V4 = %+v", status.V4)
	}
	if status.V6.State != "no-route" {
		t.Errorf("V6 = %+v", status.V6)
	}
	if len(run.Commands) != 1 || !strings.HasSuffix(run.Commands[0], " --status") {
		t.Errorf("commands = %v, want the one status call", run.Commands)
	}
}
