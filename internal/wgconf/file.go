package wgconf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"amneziawg-web-ui/internal/atomicfile"
	"amneziawg-web-ui/web-ui/api"
)

// The operations on a server's .conf on disk. Each one is a single edit, so
// the caller can hold its own lock across the edit and the matching change
// to its in-memory state, and the file and that state cannot disagree.

// WriteServerConf creates a server's .conf from its [Interface] section.
//
// The write is atomic: a server .conf is the only copy of that server's
// private key, and a half-written one - power, a killed process, a full
// volume - would leave a tunnel that cannot come back up.
func WriteServerConf(path string, iface ServerInterface) error {
	if err := atomicfile.Write(path, []byte(iface.Render()), 0o600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	return nil
}

// AppendPeer adds a client's peer block to the end of the server's .conf.
func AppendPeer(path string, client *api.Client, allowedIPs string) error {
	return appendText(path, PeerBlock(client, allowedIPs))
}

// RemovePeer deletes the client's peer block from the server's .conf and
// returns the block it removed, so a caller that means to park it (suspend)
// does not have to parse the file a second time. A client without a block is
// not an error: nil, nil.
func RemovePeer(path string, client *api.Client) (block []string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	rest, block, found := SplitPeerBlock(strings.Split(string(data), "\n"), client)
	if !found {
		return nil, nil
	}
	if err := atomicfile.Write(path, []byte(strings.Join(rest, "\n")+"\n"), 0o600); err != nil {
		return nil, err
	}
	return block, nil
}

// RewriteInterface replaces the [Interface] section of path and keeps every
// peer block after it.
//
// It is how a setting of a server that already exists - the MTU, above all -
// reaches the .conf: recreating the interface section from the model is
// exactly what WriteServerConf does, but writing that over a file that also
// holds the peers would drop every client on the way. The peers are the one
// part of the file the model does not describe, so they are copied through
// untouched.
func RewriteInterface(path string, iface ServerInterface) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	start := -1
	for i, line := range lines {
		if _, _, isMarker := ParsePeerMarker(line); isMarker || startsPeerBlock(line) {
			start = i
			break
		}
	}

	body := iface.Render()
	if start >= 0 {
		// Peer blocks are introduced by a blank line, so the rewritten
		// section ends with one before the first of them.
		body += "\n" + strings.Join(lines[start:], "\n") + "\n"
	}
	if err := atomicfile.Write(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("rewriting %s: %w", path, err)
	}
	return nil
}

// SuspendedDir is where a suspended peer block is parked while it is out of
// the server's live .conf. It sits next to that .conf rather than under a
// fixed path, so the two always move together.
func SuspendedDir(serverConfPath string) string {
	return filepath.Join(filepath.Dir(serverConfPath), "suspended")
}

// ParkPeer takes the client's block out of the server's .conf and stores it
// under SuspendedDir, to be restored by RestorePeer.
//
// The parked copy is written before the live file is rewritten: the other way
// round, a parked file that could not be written left the peer out of the live
// config with no copy anywhere until the next start rebuilt it from memory.
func ParkPeer(serverConfPath string, client *api.Client) error {
	data, err := os.ReadFile(serverConfPath)
	if err != nil {
		return fmt.Errorf("rewriting %s: %w", serverConfPath, err)
	}
	rest, block, found := SplitPeerBlock(strings.Split(string(data), "\n"), client)
	if !found {
		return nil
	}

	dir := SuspendedDir(serverConfPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("storing the suspended peer: %w", err)
	}
	parked := filepath.Join(dir, client.ID+".conf")
	if err := atomicfile.Write(parked, []byte(strings.Join(block, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("storing the suspended peer: %w", err)
	}

	if err := atomicfile.Write(serverConfPath, []byte(strings.Join(rest, "\n")+"\n"), 0o600); err != nil {
		// The live file was not touched, so the copy just written is not a
		// park at all - the client is still active in the config. Drop it
		// rather than leave a block that a later activation would find.
		os.Remove(parked)
		return fmt.Errorf("rewriting %s: %w", serverConfPath, err)
	}
	return nil
}

// RestorePeer appends the client's parked block back onto the server's .conf
// and removes the parked copy. The client may have been renamed while it was
// parked, so the marker is rewritten from the client as it is now - and so are
// the keys and addresses, after checking that the block really is this
// client's.
func RestorePeer(serverConfPath string, client *api.Client) error {
	parked := filepath.Join(SuspendedDir(serverConfPath), client.ID+".conf")
	suspended, err := os.ReadFile(parked)
	if err != nil {
		return fmt.Errorf("reading the suspended peer of %s: %w", client.ID, err)
	}

	block, err := RewritePeerForClient(strings.Split(strings.TrimRight(string(suspended), "\n"), "\n"), client)
	if err != nil {
		return fmt.Errorf("refusing to bring %s back: %w", client.ID, err)
	}
	if err := appendText(serverConfPath, "\n"+strings.Join(block, "\n")+"\n"); err != nil {
		return err
	}
	os.Remove(parked)
	return nil
}

func appendText(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("reopening %s: %w", path, err)
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return fmt.Errorf("appending to %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
