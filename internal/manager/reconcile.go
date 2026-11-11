package manager

import (
	"fmt"

	"amneziawg-web-ui/internal/wgconf"
)

// reconcilePeers settles the disagreement a crash can leave between a server's
// .conf and the clients the panel knows about. wgconf.Reconcile documents the
// direction: the model is the truth, the file is derived from it, and a peer
// the model does not know is parked rather than deleted.
func (m *Manager) reconcilePeers() {
	for _, srv := range m.copyServers() {
		result, err := wgconf.Reconcile(srv.ConfigPath, srv.Clients)
		if err != nil {
			fmt.Printf("Reconcile %s: %v\n", srv.Name, err)
			continue
		}
		if !result.Changed() {
			continue
		}

		fmt.Printf("Reconcile %s: %s\n", srv.Name, result)
		// A server that is already up outside this process (a host-network
		// deployment whose panel restarted) is still running the old peer
		// list; syncconf is what makes the change take effect.
		m.syncLiveConfig(srv.Interface)
	}
}
