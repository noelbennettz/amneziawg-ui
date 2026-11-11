package wgconf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"amneziawg-web-ui/internal/atomicfile"
	"amneziawg-web-ui/web-ui/api"
)

// The .conf and the config store are two files written one after the other, so
// a crash between them leaves them disagreeing. Reconcile is how the process
// settles that at start-up, and the direction it settles in is the whole
// design: the model is the truth, the file is derived from it.
//
// A peer the model knows about but the file lost is written back - its keys
// and addresses are in the model, so the block can be rebuilt exactly. A peer
// the file has and the model does not is parked rather than deleted: the panel
// owns this file, so an unknown peer is either a client whose deletion was
// interrupted or something somebody put there by hand, and neither is worth
// throwing away without a copy. Parked orphans land under a name that
// activation never picks up (that one is `<client id>.conf`), so they cannot
// silently come back.

// ReconcileResult says what a reconcile pass had to change.
type ReconcileResult struct {
	// Added are clients the model has as active and the file had lost.
	Added []string
	// Parked are blocks the file had and the model says are suspended.
	Parked []string
	// Orphans are blocks whose client the model does not know at all.
	Orphans []string
	// Unattributed are hand-written peer blocks: they carry no client marker,
	// so nothing here can say whose they are. They are left where they are
	// and reported.
	Unattributed int
}

// Changed reports whether the pass had anything to do.
func (r ReconcileResult) Changed() bool {
	return len(r.Added) > 0 || len(r.Parked) > 0 || len(r.Orphans) > 0
}

// String is the one line the start-up log prints when a pass had work to do.
func (r ReconcileResult) String() string {
	var parts []string
	if len(r.Added) > 0 {
		parts = append(parts, fmt.Sprintf("restored %s", strings.Join(r.Added, ", ")))
	}
	if len(r.Parked) > 0 {
		parts = append(parts, fmt.Sprintf("parked %s", strings.Join(r.Parked, ", ")))
	}
	if len(r.Orphans) > 0 {
		parts = append(parts, fmt.Sprintf("parked unknown peer(s) %s", strings.Join(r.Orphans, ", ")))
	}
	if r.Unattributed > 0 {
		parts = append(parts, fmt.Sprintf("left %d hand-written peer block(s) alone", r.Unattributed))
	}
	return strings.Join(parts, "; ")
}

// Reconcile brings one server's .conf in line with the clients the model has.
// It writes the file only when something changed, and that write is atomic.
func Reconcile(path string, clients []api.Client) (ReconcileResult, error) {
	var result ReconcileResult

	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	byID := make(map[string]*api.Client, len(clients))
	for i := range clients {
		byID[clients[i].ID] = &clients[i]
	}

	kept := make([]string, 0, len(lines))
	seen := make(map[string]bool, len(clients))
	changed := false

	for i := 0; i < len(lines); {
		name, id, isMarker := ParsePeerMarker(lines[i])
		if !isMarker {
			// A peer block nobody marked is hand-written, or older than the
			// markers. It is copied through untouched and counted: nothing here
			// can say whose it is, and guessing would delete somebody's peer.
			if strings.TrimSpace(lines[i]) == "[Peer]" {
				block, next := peerBlockAt(lines, i)
				result.Unattributed++
				kept = append(kept, block...)
				i = next
				continue
			}
			kept = append(kept, lines[i])
			i++
			continue
		}

		block, next := peerBlockAt(lines, i)

		switch {
		case id == "":
			// A marker with no client ID matches nobody.
			result.Unattributed++
			kept = append(kept, block...)
		case byID[id] == nil:
			// The model has no such client. Park it under a name activation
			// never looks for, so it can be fetched back by hand.
			if err := ParkLines(path, "orphan-"+id, block); err != nil {
				return result, err
			}
			result.Orphans = append(result.Orphans, name+" ("+id+")")
			changed = true
		case byID[id].Status != "active":
			// The model says suspended: the block belongs in the parked
			// directory under the client's own name, which is where
			// ActivateClient reads it from.
			if err := ParkLines(path, id, block); err != nil {
				return result, err
			}
			result.Parked = append(result.Parked, name)
			changed = true
		default:
			seen[id] = true
			kept = append(kept, block...)
		}
		i = next
	}

	// Clients the model has as active that the file lost: rebuild the block
	// from the model, which is where their keys and addresses live.
	for i := range clients {
		c := &clients[i]
		if c.Status != "active" || seen[c.ID] {
			continue
		}
		kept = append(kept, strings.Split(strings.TrimRight(PeerBlock(c, PeerAllowedIPs(c)), "\n"), "\n")...)
		result.Added = append(result.Added, c.Name)
		changed = true
	}

	if !changed {
		return result, nil
	}
	return result, atomicfile.Write(path, []byte(strings.Join(kept, "\n")+"\n"), 0o600)
}

// peerBlockAt returns the block that starts at i and the index just past it. A
// block runs from its marker (or its [Peer] line) to the line before the next
// marker, trailing blank lines excluded, so removing one cannot swallow the
// peer that follows.
func peerBlockAt(lines []string, i int) ([]string, int) {
	end := len(lines)
	for j := i + 1; j < len(lines); j++ {
		if _, _, next := ParsePeerMarker(lines[j]); next {
			end = j
			break
		}
	}
	return trimTrailingBlank(lines[i:end]), end
}

// ParkLines stores one peer block under SuspendedDir so it is out of the live
// config but not lost. key is the file name without the extension.
func ParkLines(serverConfPath, key string, block []string) error {
	dir := SuspendedDir(serverConfPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("storing the parked peer: %w", err)
	}
	path := filepath.Join(dir, key+".conf")
	if err := atomicfile.Write(path, []byte(strings.Join(block, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("storing the parked peer: %w", err)
	}
	return nil
}

// ForgetPeer removes a client's parked block. Deletion needs it and activation
// does not: a client that no longer exists must not leave its peer - public
// key, preshared key and allowed addresses - behind on the volume.
func ForgetPeer(serverConfPath, clientID string) error {
	path := filepath.Join(SuspendedDir(serverConfPath), clientID+".conf")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
