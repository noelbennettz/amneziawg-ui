package manager

import (
	"strconv"
	"strings"
	"time"

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

// IPTablesCheck reports which of a server's firewall rules are in place.
func (m *Manager) IPTablesCheck(serverID string) (api.IptablesTest, error) {
	srv, ok := m.Server(serverID)
	if !ok {
		return api.IptablesTest{}, serverNotFound(serverID)
	}
	return api.IptablesTest{
		ServerID:      serverID,
		ServerName:    srv.Name,
		Interface:     srv.Interface,
		Subnet:        srv.Subnet,
		SubnetV6:      srv.SubnetV6,
		IptablesCheck: m.tools.CheckIPTables(srv.Interface, srv.Subnet, srv.SubnetV6),
	}, nil
}
