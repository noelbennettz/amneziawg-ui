package store

import (
	"amneziawg-web-ui/internal/netutil"
	"amneziawg-web-ui/web-ui/api"
)

// This file holds the one-shot migrations that bring a config written by an
// older release - web_config.json and the .conf files it points at - up to
// what the current code expects. They run at startup, from manager.New,
// before anything reads the config for real.
//
// To add one:
//
//   - bump SchemaVersion to the revision the migration produces and describe
//     that revision in the list on it;
//   - give the migration its own constant holding that same revision, open
//     its body with "if cfg.SchemaVersion >= <that constant> { return }" so a
//     later bump cannot re-run it, and call it from Migrate.

// SchemaVersion is the current web_config.json schema revision.
//
//	3 - the shape the first release writes: every client lives in the Clients
//	    list of its server, and every peer marker in a server .conf carries
//	    the client ID.
//	4 - every client's ServerID actually names the server it lives in; see
//	    repairClientServerIDs.
//	5 - every server owns a ULA /64 and every client an address in it; see
//	    addIPv6Addressing.
//
// It starts at 3 rather than 1 because pre-release builds already stamped 1
// and 2 into deployed configs. Numbering the first release below what is
// already on disk would make those files look newer than the code, and the
// next migration - guarded by "the config is older than this revision" -
// would never run on them.
const SchemaVersion = 5

// Migrate applies every migration cfg still needs and stamps the result, so
// none of them run twice. It reports whether cfg changed and therefore needs
// saving; a config that is already current is left untouched.
func Migrate(cfg *AppConfig) (changed bool) {
	if cfg.SchemaVersion >= SchemaVersion {
		return false
	}

	// Migrations go here, oldest first.
	repairClientServerIDs(cfg)
	addIPv6Addressing(cfg)

	cfg.SchemaVersion = SchemaVersion
	return true
}

const schemaClientServerIDs = 4

// repairClientServerIDs rewrites every client's ServerID from the server it is
// stored under.
//
// Releases up to 3.1.4 took that id straight from the route parameter and kept
// it, but fiber hands route parameters back as strings pointing into the
// request buffer it recycles the moment the handler returns. The id therefore
// decayed into a slice of some later request ("tatus5" off a /api/system/status
// path, "i-sett" off an .../i-settings one) and any save after that wrote the
// garbage to disk. Membership in the server's Clients list is what actually
// binds a client to its server, so the correct value is always to hand and
// nothing else needs repairing.
func repairClientServerIDs(cfg *AppConfig) {
	if cfg.SchemaVersion >= schemaClientServerIDs {
		return
	}

	for i := range cfg.Servers {
		srv := &cfg.Servers[i]
		for j := range srv.Clients {
			if srv.Clients[j].ServerID != srv.ID {
				srv.Clients[j].ServerID = srv.ID
			}
		}
	}
}

const schemaIPv6 = 5

// addIPv6Addressing gives every server a ULA /64 and every client an address
// in it, so a config written by a release that only knew about IPv4 comes up
// dual-stack.
//
// The addresses are only added to the config here; the manager fixes the .conf
// files the servers were created with on the next start (see
// Manager.reconcileIPv6Conf), because rendering them is that package's job.
// Existing clients keep the addresses they were handed out in order - a client
// that already has one is not moved.
func addIPv6Addressing(cfg *AppConfig) {
	if cfg.SchemaVersion >= schemaIPv6 {
		return
	}

	for i := range cfg.Servers {
		srv := &cfg.Servers[i]

		if srv.SubnetV6 == "" {
			srv.SubnetV6 = api.NextFreeSubnetV6(srv.ID, func(candidate string) bool {
				for j := range cfg.Servers {
					if j != i && api.SubnetsOverlap(candidate, cfg.Servers[j].SubnetV6) {
						return true
					}
				}
				return false
			})
		}
		if srv.SubnetV6 == "" {
			continue
		}

		if srv.ServerIPV6 == "" {
			network, _ := netutil.SplitCIDR(srv.SubnetV6)
			srv.ServerIPV6 = netutil.ServerIPV6(network)
		}

		used := map[string]bool{srv.ServerIPV6: true}
		for j := range srv.Clients {
			if addr := srv.Clients[j].ClientIPV6; addr != "" {
				used[addr] = true
			}
		}
		for j := range srv.Clients {
			if srv.Clients[j].ClientIPV6 != "" {
				continue
			}
			addr := netutil.NextFreeIPv6(srv.SubnetV6, used)
			if addr == "" {
				break
			}
			srv.Clients[j].ClientIPV6 = addr
			used[addr] = true
		}

		if srv.UnboundNATIPsV6 == nil {
			srv.UnboundNATIPsV6 = []string{}
		}
	}
}
