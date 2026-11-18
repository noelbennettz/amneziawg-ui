package ui

import (
	"testing"

	"amneziawg-web-ui/web-ui/internal/ui/newserver"
)

// A page that has not heard from /api/system/status yet has to answer with the
// same values the backend would use by default, and it must not read its own
// zero state as "the operator turned auto-start off": false is a real answer
// here, which is why the state remembers whether an answer ever arrived.
func TestDefaultsBeforeTheStatusArrives(t *testing.T) {
	s := newState()

	if !s.DefaultAutoStart() {
		t.Error("a fresh page reports AUTO_START_SERVERS=false")
	}
	if got := s.DefaultPort(); got != newserver.DefaultPort {
		t.Errorf("DefaultPort() = %d, want the built-in %d", got, newserver.DefaultPort)
	}
	if got := s.DefaultSubnet(); got != newserver.DefaultSubnet {
		t.Errorf("DefaultSubnet() = %q, want the built-in %q", got, newserver.DefaultSubnet)
	}
	if got := s.DefaultDNS(); got != newserver.DefaultDNS {
		t.Errorf("DefaultDNS() = %q, want the built-in %q", got, newserver.DefaultDNS)
	}
	if got := s.ServerMTU(); got != newserver.DefaultMTU {
		t.Errorf("ServerMTU() = %d, want the built-in %d", got, newserver.DefaultMTU)
	}
}

// The four answers the create form starts from are the ones the status call
// carried, once it has landed - and a reported "off" is answered as off.
func TestDefaultsComeFromTheStatus(t *testing.T) {
	s := newState()
	s.mu.Lock()
	s.serverDefaultMTU = 1300
	s.serverDefaultPort = 51998
	s.serverDefaultSubnet = "10.9.0.0/24"
	s.serverDefaultDNS = "9.9.9.9"
	s.serverDefaultAutoStart = false
	s.autoStartSeen = true
	s.mu.Unlock()

	if got := s.ServerMTU(); got != 1300 {
		t.Errorf("ServerMTU() = %d, want 1300", got)
	}
	if got := s.DefaultPort(); got != 51998 {
		t.Errorf("DefaultPort() = %d, want 51998", got)
	}
	if got := s.DefaultSubnet(); got != "10.9.0.0/24" {
		t.Errorf("DefaultSubnet() = %q, want 10.9.0.0/24", got)
	}
	if got := s.DefaultDNS(); got != "9.9.9.9" {
		t.Errorf("DefaultDNS() = %q, want 9.9.9.9", got)
	}
	if s.DefaultAutoStart() {
		t.Error("DefaultAutoStart() = true, want the reported false")
	}
}

// A value the backend reported but a server may not use - an MTU or a port
// outside the range in api - falls back to the built-in default rather than
// reaching the form.
func TestUnusableDefaultsFallBack(t *testing.T) {
	s := newState()
	s.mu.Lock()
	s.serverDefaultMTU = 9000
	s.serverDefaultPort = 0
	s.mu.Unlock()

	if got := s.ServerMTU(); got != newserver.DefaultMTU {
		t.Errorf("ServerMTU() = %d, want the built-in %d", got, newserver.DefaultMTU)
	}
	if got := s.DefaultPort(); got != newserver.DefaultPort {
		t.Errorf("DefaultPort() = %d, want the built-in %d", got, newserver.DefaultPort)
	}
}
