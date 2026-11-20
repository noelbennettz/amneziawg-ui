package manager

import (
	"fmt"

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

// detectPublicIP runs the detector. A Manager built by New always has one;
// the fallback keeps a test that builds the struct by hand from calling a nil
// function.
func (m *Manager) detectPublicIP() string {
	if m.detect != nil {
		return m.detect()
	}
	return publicip.Detect(m.tools.RouteSourceIP, m.tools.RouteSourceIP6)
}

// RefreshPublicIP re-detects the public IP and stamps it on every server.
// Nothing is written when the answer is the one already stored: this is
// reachable from the page, and a read should not rewrite the config file - nor
// should a prefetch or a speculative request.
//
// A detection that fails keeps the address already in force and reports
// ErrUnavailable. Overwriting it with the YOUR_SERVER_IP placeholder would
// quietly break every config generated afterwards, including the one an
// operator is in the middle of handing out.
func (m *Manager) RefreshPublicIP() (string, error) {
	ip := m.detectPublicIP()
	if ip == publicip.Unknown {
		return m.PublicIP(), fmt.Errorf("no public address detected: the address already in force is kept: %w", ErrUnavailable)
	}

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
	return ip, nil
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
