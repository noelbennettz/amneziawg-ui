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
// Nothing is written when the answer is the one already stored: this is
// reachable as a GET, and a read should not rewrite the config file - nor
// should a prefetch, a scanner or a browser's speculative request.
func (m *Manager) RefreshPublicIP() string {
	ip := publicip.Detect(m.tools.RouteSourceIP, m.tools.RouteSourceIP6)

	m.mu.Lock()
	changed := ip != m.publicIP
	m.publicIP = ip
	for i := range m.cfg.Servers {
		if m.cfg.Servers[i].PublicIP != ip {
			m.cfg.Servers[i].PublicIP = ip
			changed = true
		}
	}
	m.mu.Unlock()

	if changed {
		m.saveOrLog("public IP")
	}
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
