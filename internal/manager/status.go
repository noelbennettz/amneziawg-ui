package manager

import (
	"fmt"
	"time"
)

// statusTTL is how long an observed interface state is reused. Long enough to
// collapse the burst of lookups one dashboard refresh makes, short enough that
// an interface going down is noticed within a poll or two.
const statusTTL = 2 * time.Second

type statusObservation struct {
	status string
	at     time.Time
	// since is when the interface was last seen coming up: the moment this
	// code brought it up, or the first observation that found it running.
	// Zero unless status is "running".
	since time.Time
}

// serverStatus reports whether the server's interface is up, from a short
// lived cache. The kernel is the only authority on this - the Status field in
// the config is a stale echo of the last observation - but every dashboard
// poll asks about every server, and the traffic poll asks again every few
// seconds, so the answer is reused for statusTTL rather than shelling out per
// caller. It shells out on a miss, so it must never be called with mu held.
func (m *Manager) serverStatus(iface string) string {
	if iface == "" {
		return "stopped"
	}
	if status, ok := m.cachedStatus(iface); ok {
		return status
	}

	status := "stopped"
	if m.tools.InterfaceUp(iface) {
		status = "running"
	}
	m.noteServerStatus(iface, status)
	return status
}

func (m *Manager) cachedStatus(iface string) (string, bool) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	seen, ok := m.statuses[iface]
	if !ok || time.Since(seen.at) > statusTTL {
		return "", false
	}
	return seen.status, true
}

// noteServerStatus records a status this code just caused, so the next reader
// does not have to wait out the cache or race the kernel. It also keeps the
// moment the interface came up, which is what interfaceUptime measures from:
// a running interface seen running again keeps its start, anything else
// resets it.
func (m *Manager) noteServerStatus(iface, status string) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	now := time.Now()
	seen := statusObservation{status: status, at: now}
	if status == "running" {
		seen.since = now
		if prev, ok := m.statuses[iface]; ok && prev.status == "running" {
			seen.since = prev.since
		}
	}
	m.statuses[iface] = seen
}

// interfaceUptime is how long the interface has been up, in seconds, as far
// as this process has seen it: zero for one that is down or never observed.
// The record outlives statusTTL on purpose - the cache expiring is not the
// interface going down; that is noted when the next lookup finds it so.
func (m *Manager) interfaceUptime(iface string) float64 {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	seen, ok := m.statuses[iface]
	if !ok || seen.status != "running" {
		return 0
	}
	return time.Since(seen.since).Seconds()
}

// forgetServerStatus drops a cached observation, for an interface that no
// longer exists.
func (m *Manager) forgetServerStatus(iface string) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	delete(m.statuses, iface)
}

// syncLiveConfig pushes the .conf onto the interface when it is actually up.
// The check is deliberately made here, after the config change and outside
// the lock, rather than from the stored Status: a peer added while the stored
// value says "stopped" but the interface is up would silently not take effect
// until the next restart.
//
// The error matters to the caller: the config change is already on disk, but
// the running tunnel still has the old one. Reporting success there would tell
// the operator a client was added (or suspended) while the kernel kept the old
// peer list until a restart.
func (m *Manager) syncLiveConfig(iface string) error {
	if m.serverStatus(iface) != "running" {
		return nil
	}
	if err := m.tools.SyncConf(iface); err != nil {
		fmt.Printf("Failed to apply live config to %s: %v\n", iface, err)
		return fmt.Errorf("applying the config to %s: %w", iface, err)
	}
	fmt.Printf("Live config applied to %s\n", iface)
	return nil
}

// notApplied wraps a live-sync failure for an API caller. The change is
// stored; what failed is pushing it onto the interface that is already up.
func notApplied(err error) error {
	return fmt.Errorf("the change was saved, but it could not be applied to the running interface (restart the server to pick it up): %w", err)
}
