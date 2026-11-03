package manager

import (
	"fmt"

	"amneziawg-web-ui/internal/netutil"
	"amneziawg-web-ui/internal/wgconf"
)

// reconcileIPv6Conf brings the .conf files written before IPv6 support up to
// what the config in memory says: the server's own address in its ULA /64 and
// the IPv6 address of every client. The store migration fills the addresses
// in, but the files are rendered from them only when a peer is added and the
// [Interface] section is written once at creation, so an upgraded install
// would otherwise keep handing out dual-stack client configs for a server that
// answers on IPv4 only.
//
// It runs on every start and writes only where something is missing: a file
// that already carries the addresses is read and left alone.
func (m *Manager) reconcileIPv6Conf() {
	for _, srv := range m.copyServers() {
		if srv.SubnetV6 == "" || srv.ServerIPV6 == "" {
			continue
		}

		_, prefixV6 := netutil.SplitCIDR(srv.SubnetV6)
		if err := wgconf.EnsureInterfaceAddress(srv.ConfigPath, srv.ServerIPV6+"/"+prefixV6); err != nil {
			fmt.Printf("Adding the IPv6 address to %s: %v\n", srv.ConfigPath, err)
			continue
		}

		for i := range srv.Clients {
			client := &srv.Clients[i]
			if client.ClientIPV6 == "" {
				continue
			}
			if err := wgconf.EnsurePeerAllowedIPs(srv.ConfigPath, client, wgconf.PeerAllowedIPs(client)); err != nil {
				fmt.Printf("Adding IPv6 address of client %s to %s: %v\n", client.ID, srv.ConfigPath, err)
			}
		}
	}
}
