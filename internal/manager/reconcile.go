package manager

import (
	"fmt"
	"os"

	"amneziawg-web-ui/internal/netutil"
	"amneziawg-web-ui/internal/wgconf"
	"amneziawg-web-ui/web-ui/api"
)

// reconcilePeers settles the disagreement a crash can leave between a server's
// .conf and the clients the panel knows about. wgconf.Reconcile documents the
// direction: the model is the truth, the file is derived from it, and a peer
// the model does not know is parked rather than deleted.
func (m *Manager) reconcilePeers() {
	for _, srv := range m.copyServers() {
		if err := m.ensureServerConf(srv); err != nil {
			fmt.Printf("Reconcile %s: %v\n", srv.Name, err)
			continue
		}
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

// ensureServerConf writes a server's .conf back when the file is gone.
//
// The .conf is derived: its interface section is the model's private key,
// addresses, port, MTU and obfuscation parameters, and its peers are the
// model's active clients. A deleted or lost file therefore costs nothing to
// rebuild, and a server that cannot find its file is otherwise one that can
// never be started again - every start fails on the missing path, with no way
// back through the panel.
//
// A file that exists is left alone, whatever it contains: Reconcile takes over
// from there, and the peer blocks are the part that can legitimately differ.
func (m *Manager) ensureServerConf(srv api.Server) error {
	switch _, err := os.Stat(srv.ConfigPath); {
	case err == nil:
		return nil
	case !os.IsNotExist(err):
		return fmt.Errorf("checking %s: %w", srv.ConfigPath, err)
	}

	if srv.ServerPrivateKey == "" {
		return fmt.Errorf("%s is missing and the config has no private key for that server, so it cannot be rebuilt", srv.ConfigPath)
	}
	if srv.ServerIP == "" || srv.Subnet == "" {
		return fmt.Errorf("%s is missing and the config has no address for that server, so it cannot be rebuilt", srv.ConfigPath)
	}

	_, prefix := netutil.SplitCIDR(srv.Subnet)
	iface := wgconf.ServerInterface{
		PrivateKey: srv.ServerPrivateKey,
		Address:    srv.ServerIP + "/" + prefix,
		ListenPort: srv.Port,
		MTU:        srv.MTU,
	}
	if srv.SubnetV6 != "" && srv.ServerIPV6 != "" {
		_, prefixV6 := netutil.SplitCIDR(srv.SubnetV6)
		iface.AddressV6 = srv.ServerIPV6 + "/" + prefixV6
	}
	if srv.ObfuscationEnabled {
		iface.Obfuscation = srv.ObfuscationParams
	}

	if err := wgconf.WriteServerConf(srv.ConfigPath, iface); err != nil {
		return fmt.Errorf("rebuilding %s: %w", srv.ConfigPath, err)
	}
	fmt.Printf("Rebuilt %s from the config: the file was missing\n", srv.ConfigPath)
	return nil
}
