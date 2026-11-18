package manager

import (
	"strconv"
	"strings"
	"time"

	"amneziawg-web-ui/internal/awg"
	"amneziawg-web-ui/web-ui/api"
)

// SystemStatus is the backend's own health: what is installed, how many
// servers and clients there are, and the defaults it started with.
func (m *Manager) SystemStatus() api.SystemStatus {
	servers := m.Servers()
	active := 0
	for _, s := range servers {
		if s.Status == "running" {
			active++
		}
	}

	return api.SystemStatus{
		AWGAvailable:  m.tools.Available(),
		PublicIP:      m.PublicIP(),
		TotalServers:  len(servers),
		TotalClients:  m.ClientCount(),
		ActiveServers: active,
		UptimeSeconds: time.Since(m.started).Seconds(),
		Timestamp:     float64(time.Now().Unix()),
		Environment: api.SystemEnvironment{
			WebUIPort:        strconv.Itoa(m.settings.WebUIPort),
			AutoStartServers: m.settings.AutoStart,
			DefaultMTU:       m.settings.DefaultMTU,
			DefaultSubnet:    m.settings.DefaultSubnet,
			DefaultPort:      m.settings.DefaultPort,
			DefaultDNS:       strings.Join(m.settings.DNSServers, ","),
		},
	}
}

// IPTablesCheck reports which of a server's firewall rules are in place, and
// what the firewall script decided about each family's way out.
func (m *Manager) IPTablesCheck(serverID string) (api.IptablesTest, error) {
	srv, ok := m.Server(serverID)
	if !ok {
		return api.IptablesTest{}, serverNotFound(serverID)
	}
	check, firewall := m.tools.CheckIPTables(srv.Interface, srv.Subnet, srv.SubnetV6)
	return api.IptablesTest{
		ServerID:      serverID,
		ServerName:    srv.Name,
		Interface:     srv.Interface,
		Subnet:        srv.Subnet,
		SubnetV6:      srv.SubnetV6,
		IptablesCheck: check,
		NAT4:          natStatus(firewall.V4),
		NAT6:          natStatus(firewall.V6),
	}, nil
}

// natStatus carries one family's verdict into the API. "unknown" is the
// script not answering - not deployed, or failed - which is not the same as a
// family that has no route and no forwarding rules.
func natStatus(st awg.NATStatus) api.NATStatus {
	state := st.State
	if state == "" {
		state = "unknown"
	}
	return api.NATStatus{State: state, Uplink: st.Uplink, Source: st.Source}
}
