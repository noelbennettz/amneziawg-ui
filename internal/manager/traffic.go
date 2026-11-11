package manager

import (
	"time"

	"amneziawg-web-ui/internal/awg"
	"amneziawg-web-ui/web-ui/api"
)

// trafficCacheTTL is how long one snapshot answers for. The panel polls every
// five seconds per tab, and several tabs watch the same counters: sharing an
// answer for a moment turns N tabs' worth of awg runs into one. The counters
// are cumulative, so a snapshot a second or two old only shifts the rate graph
// by that much - it cannot invent traffic that is not there.
const trafficCacheTTL = 3 * time.Second

// TrafficSnapshot returns the peer counters of every server, plus the host's
// own gauges, from the cache when it is fresh enough.
func (m *Manager) TrafficSnapshot() api.TrafficSnapshot {
	m.trafficMu.Lock()
	defer m.trafficMu.Unlock()

	if !m.trafficAt.IsZero() && time.Since(m.trafficAt) < trafficCacheTTL {
		return m.traffic
	}

	snapshot := m.collectTraffic()
	m.traffic, m.trafficAt = snapshot, time.Now()
	return snapshot
}

// collectTraffic walks every server and asks awg for its peers. Servers that
// are down contribute nothing; the page keeps their last counters as they
// were.
func (m *Manager) collectTraffic() api.TrafficSnapshot {
	servers := m.copyServers()

	clientTraffic := map[string]map[string]api.ClientTraffic{}
	serverTraffic := map[string]api.InterfaceTraffic{}
	for i := range servers {
		srv := &servers[i]
		peers := m.tools.ShowPeers(srv.Interface)
		if peers == nil {
			continue
		}
		// Peers coming back is the interface being up, and this poll is
		// the most frequent one: noting it here is what keeps the uptime
		// running from the first sighting.
		m.noteServerStatus(srv.Interface, "running")
		if t := peerTraffic(srv, peers); len(t) > 0 {
			clientTraffic[srv.ID] = t
		}
		var rx, tx uint64
		for _, p := range peers {
			rx += p.RXBytes
			tx += p.TXBytes
		}
		serverTraffic[srv.ID] = api.InterfaceTraffic{
			RX: api.FormatBytes(float64(rx)), TX: api.FormatBytes(float64(tx)),
			RXBytes: rx, TXBytes: tx,
			UptimeSeconds: m.interfaceUptime(srv.Interface),
		}
	}

	return api.TrafficSnapshot{
		Timestamp:     float64(time.Now().Unix()),
		ClientTraffic: clientTraffic,
		ServerTraffic: serverTraffic,
		System:        m.host.Metrics(),
	}
}

// onlineWindow is how old a handshake may be for the peer to count as
// online. WireGuard renews the session every two minutes while traffic
// flows and drops the keys after three (REJECT_AFTER_TIME): a peer quiet for
// longer than that has no live session.
const onlineWindow = 3 * time.Minute

// peerTraffic joins what `awg show` reports with the clients the server
// owns, keyed by client ID. srv is a snapshot, so no lock is needed.
func peerTraffic(srv *api.Server, peers map[string]awg.PeerStats) map[string]api.ClientTraffic {
	result := map[string]api.ClientTraffic{}
	now := time.Now()
	for _, c := range srv.Clients {
		p, ok := peers[c.ClientPublicKey]
		if !ok {
			p = awg.PeerStats{LastHandshake: "Never"}
		}
		result[c.ID] = api.ClientTraffic{
			Received:      api.FormatBytes(float64(p.RXBytes)),
			Sent:          api.FormatBytes(float64(p.TXBytes)),
			LastHandshake: p.LastHandshake,
			Endpoint:      p.Endpoint,
			Online:        !p.HandshakeAt.IsZero() && now.Sub(p.HandshakeAt) < onlineWindow,
		}
	}
	return result
}
