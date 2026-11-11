package ui

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/browser"
	"amneziawg-web-ui/web-ui/internal/ui/newserver"
	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

const (
	// The page polls: the counters often, since that is what the operator
	// watches move, and the server list rarely - every action that changes
	// it reloads it directly, so the loop only catches changes made from
	// another tab.
	trafficInterval = 5 * time.Second
	serversInterval = 30 * time.Second
	// statusInterval is the same beat as the server list. The status call is
	// what carries the defaults the create form starts from, so it cannot be
	// a once-per-page-load thing: an operator who edits DEFAULT_PORT and
	// restarts the container has a page open that would otherwise keep the
	// old numbers for good - and would create servers with them.
	statusInterval = serversInterval
)

// state is what the page has last heard from the backend, behind one lock:
// the server list the cards are rendered from, and the defaults the create
// form and the client editor start from. It is the create form's
// newserver.State.
type state struct {
	mu          sync.Mutex
	servers     []api.Server
	serversHash string
	defaultI    api.ISettings

	// webUIPort is the panel's own listener, reported by /api/system/status.
	// The create form treats it as taken like any server's port.
	webUIPort int

	// serverDefaultMTU is DEFAULT_MTU as the backend was started with it,
	// also from /api/system/status. The create form starts from it rather
	// than from a number of its own, so the operator's setting is what a new
	// server actually gets. The other three are the same for DEFAULT_PORT,
	// DEFAULT_SUBNET and DEFAULT_DNS.
	serverDefaultMTU    int
	serverDefaultPort   int
	serverDefaultSubnet string
	serverDefaultDNS    string

	// serverDefaultAutoStart is AUTO_START_SERVERS, and autoStartSeen is
	// what tells a reported "off" from the zero value: until the status
	// call lands the panel keeps its own default rather than pretending the
	// operator turned starting off.
	serverDefaultAutoStart bool
	autoStartSeen          bool
}

func newState() *state {
	return &state{defaultI: api.ISettings{}}
}

// ServerMTU is the MTU a new server starts from: what the backend was
// configured with, or the built-in default while the status call is still in
// flight or reported something outside the range servers are allowed.
func (s *state) ServerMTU() int {
	s.mu.Lock()
	mtu := s.serverDefaultMTU
	s.mu.Unlock()

	if mtu < api.MinMTU || mtu > api.MaxMTU {
		return newserver.DefaultMTU
	}
	return mtu
}

// DefaultPort is the port a new server starts from: DEFAULT_PORT as the backend
// was started with it, or the built-in default while that is unknown.
func (s *state) DefaultPort() int {
	s.mu.Lock()
	port := s.serverDefaultPort
	s.mu.Unlock()

	if port < api.MinPort || port > api.MaxPort {
		return newserver.DefaultPort
	}
	return port
}

// DefaultSubnet is the subnet a new server starts from; see DefaultPort.
func (s *state) DefaultSubnet() string {
	s.mu.Lock()
	subnet := s.serverDefaultSubnet
	s.mu.Unlock()

	if subnet == "" {
		return newserver.DefaultSubnet
	}
	return subnet
}

// DefaultAutoStart is whether a server created here is brought up as soon as
// it exists: AUTO_START_SERVERS as the backend was started with it, or the
// built-in default while that is unknown.
func (s *state) DefaultAutoStart() bool {
	s.mu.Lock()
	autoStart, seen := s.serverDefaultAutoStart, s.autoStartSeen
	s.mu.Unlock()

	if !seen {
		return newserver.DefaultAutoStart
	}
	return autoStart
}

// DefaultDNS is the DNS a new server starts from; see DefaultPort.
func (s *state) DefaultDNS() string {
	s.mu.Lock()
	dns := s.serverDefaultDNS
	s.mu.Unlock()

	if dns == "" {
		return newserver.DefaultDNS
	}
	return dns
}

// TakenPorts maps every port that is already spoken for to what holds it: an
// existing server, or the panel's own listener. Two interfaces cannot share a
// port, and the clash would otherwise only show up when the second one fails
// to come up.
func (s *state) TakenPorts() map[int]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	taken := make(map[int]string, len(s.servers)+1)
	if s.webUIPort > 0 {
		taken[s.webUIPort] = lang.L("the web UI")
	}
	for _, server := range s.servers {
		taken[server.Port] = lang.L("server \"{{.Name}}\"", map[string]any{"Name": server.Name})
	}
	return taken
}

// TakenSubnets maps every subnet an existing server occupies to that server.
func (s *state) TakenSubnets() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	taken := make(map[string]string, len(s.servers))
	for _, server := range s.servers {
		taken[strings.TrimSpace(server.Subnet)] = lang.L("{{.Subnet}} of server \"{{.Name}}\"", map[string]any{"Subnet": strings.TrimSpace(server.Subnet), "Name": server.Name})
	}
	return taken
}

// TakenSubnetsV6 is TakenSubnets for the IPv6 half: every ULA /64 an existing
// server owns, mapped to that server.
func (s *state) TakenSubnetsV6() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	taken := make(map[string]string, len(s.servers))
	for _, server := range s.servers {
		subnet := strings.TrimSpace(server.SubnetV6)
		if subnet == "" {
			continue
		}
		taken[subnet] = lang.L("{{.Subnet}} of server \"{{.Name}}\"", map[string]any{"Subnet": subnet, "Name": server.Name})
	}
	return taken
}

// defaultISettings is the I1-I5 set the backend gives a new client, as last
// reported.
func (s *state) defaultISettings() api.ISettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.defaultI
}

// ── Data loading ─────────────────────────────────────────────────────────────

// Start begins loading once the toolkit is running, so the first fyne.Do
// calls always find a live event loop.
func (u *UI) Start() {
	// The first layout runs before the window knows its size on a phone, and
	// a page that came up wide and switched a frame later would flash a
	// clipped card list on every reload. One reflow up front puts it in the
	// layout the window is actually going to have.
	u.reflow(u.win.Canvas().Size().Width)

	go func() {
		for {
			if browser.Visible() {
				u.loadSystemStatus()
			}
			wait(statusInterval)
		}
	}()
	go u.loadDefaultISettings()
	go u.reloadServers()

	// The first poll goes out at once rather than after an interval: the
	// dashboard's gauges have nothing to show until it lands, and its rates
	// need a second one on top of that.
	go func() {
		for {
			if browser.Visible() {
				u.refreshTraffic()
			}
			wait(trafficInterval)
		}
	}()
	go func() {
		for {
			wait(serversInterval)
			if browser.Visible() {
				u.reloadServers()
			}
		}
	}()
}

// wait pauses for d, and returns as soon as the page is visible again, so a
// tab that comes back refreshes at once instead of waiting out the rest of the
// interval. While it stays hidden the polls are skipped altogether: every one
// of them is a set of awg commands on the server for a page nobody is looking
// at.
func wait(d time.Duration) {
	deadline := time.Now().Add(d)
	wasHidden := !browser.Visible()
	for time.Now().Before(deadline) {
		hidden := !browser.Visible()
		if wasHidden && !hidden {
			return
		}
		wasHidden = hidden
		time.Sleep(500 * time.Millisecond)
	}
}

func (u *UI) loadSystemStatus() {
	status, err := u.env.Backend.SystemStatus()
	if err != nil {
		u.unreachable()
		return
	}
	u.setTransport(lang.L("online"), style.Success)

	port, err := strconv.Atoi(strings.TrimSpace(status.Environment.WebUIPort))
	u.mu.Lock()
	if err == nil {
		u.webUIPort = port
	}
	u.serverDefaultMTU = status.Environment.DefaultMTU
	u.serverDefaultPort = status.Environment.DefaultPort
	u.serverDefaultSubnet = status.Environment.DefaultSubnet
	u.serverDefaultDNS = status.Environment.DefaultDNS
	u.serverDefaultAutoStart = status.Environment.AutoStartServers
	u.autoStartSeen = true
	u.mu.Unlock()

	// The form was built before this call landed, with the built-in defaults
	// in its fields; this is what puts DEFAULT_PORT, DEFAULT_SUBNET,
	// DEFAULT_DNS and AUTO_START_SERVERS in front of the operator - and it
	// leaves alone anything already typed over.
	fyne.Do(func() { u.form.SeedDefaults() })

	u.setSummary(lang.L("{{.Active}}/{{.Total}} servers running",
		map[string]any{"Active": status.ActiveServers, "Total": status.TotalServers}) +
		" · " + lang.N("{{.Count}} clients", status.TotalClients, map[string]any{"Count": status.TotalClients}) +
		" · " + lang.L("up {{.Uptime}}", map[string]any{"Uptime": widgets.Uptime(status.UptimeSeconds)}))
	u.setPublicIP(status.PublicIP)
	fyne.Do(func() { u.charts.SetUptime(status.UptimeSeconds) })
}

func (u *UI) loadDefaultISettings() {
	settings, err := u.env.Backend.DefaultISettings()
	if err != nil {
		return
	}
	u.mu.Lock()
	u.defaultI = settings
	u.mu.Unlock()
}

func (u *UI) refreshPublicIP() {
	go func() {
		ip, err := u.env.Backend.RefreshIP()
		if err != nil {
			u.feedback.Fail(err)
			return
		}
		u.setPublicIP(ip)
		u.feedback.OK(lang.L("Public IP refreshed"))
		u.reloadServers()
	}()
}

// reloadServers pulls the server list and rebuilds the cards when something
// actually changed. Every action that alters structure (create, delete,
// start/stop, client changes) calls it, through env.Env.Reload.
func (u *UI) reloadServers() {
	servers, err := u.env.Backend.Servers()
	if err != nil {
		u.unreachable()
		return
	}
	u.loadSystemStatus()

	hash := fingerprint(servers)

	u.mu.Lock()
	unchanged := hash == u.serversHash
	u.servers = servers
	u.serversHash = hash
	u.mu.Unlock()

	if unchanged {
		return
	}

	fyne.Do(func() {
		u.list.Render(servers)
		u.clampScroll()
	})
	u.refreshTraffic()
}

// refreshTraffic fetches the counters and hands them to the list and the
// dashboard on the UI goroutine.
func (u *UI) refreshTraffic() {
	traffic, err := u.env.Backend.Traffic()
	if err != nil {
		u.unreachable()
		return
	}
	fyne.Do(func() {
		if u.list.ApplyTraffic(traffic.ServerTraffic, traffic.ClientTraffic) {
			u.clampScroll()
		}
		u.charts.Apply(traffic)
	})
}

// fingerprint is a cheap "did anything change" marker for the server list.
func fingerprint(servers []api.Server) string {
	data, err := json.Marshal(servers)
	if err != nil {
		return ""
	}
	return string(data)
}
