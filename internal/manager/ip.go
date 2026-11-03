package manager

import (
	"amneziawg-web-ui/internal/publicip"
	"amneziawg-web-ui/web-ui/api"
)

// PublicIP returns the address every generated config points clients at.
func (m *Manager) PublicIP() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.publicIP
}

func (m *Manager) setPublicIP(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publicIP = ip
}

// RefreshPublicIP re-detects the public IP and stamps it on every server.
func (m *Manager) RefreshPublicIP() string {
	ip := publicip.Detect(m.tools.RouteSourceIP)

	m.mu.Lock()
	m.publicIP = ip
	for i := range m.cfg.Servers {
		m.cfg.Servers[i].PublicIP = ip
	}
	m.mu.Unlock()

	m.saveOrLog("public IP")
	return ip
}

// endpointFor is the host a client of srv connects to: the endpoint the
// server was created with, else the public IP stamped on it, else whatever
// the backend detected last.
func (m *Manager) endpointFor(srv *api.Server) string {
	if srv.Endpoint != "" {
		return srv.Endpoint
	}
	if srv.PublicIP != "" {
		return srv.PublicIP
	}
	return m.PublicIP()
}
