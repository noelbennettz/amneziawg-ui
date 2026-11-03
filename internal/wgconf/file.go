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
func WriteServerConf(path string, iface ServerInterface) error {
	if err := os.WriteFile(path, []byte(iface.Render()), 0o600); err != nil {
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

// SuspendedDir is where a suspended peer block is parked while it is out of
// the server's live .conf. It sits next to that .conf rather than under a
// fixed path, so the two always move together.
func SuspendedDir(serverConfPath string) string {
	return filepath.Join(filepath.Dir(serverConfPath), "suspended")
}

// ParkPeer takes the client's block out of the server's .conf and stores it
// under SuspendedDir, to be restored by RestorePeer.
func ParkPeer(serverConfPath string, client *api.Client) error {
	block, err := RemovePeer(serverConfPath, client)
	if err != nil {
		return fmt.Errorf("rewriting %s: %w", serverConfPath, err)
	}
	if len(block) == 0 {
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
	return nil
}

// RestorePeer appends the client's parked block back onto the server's .conf
// and removes the parked copy. The client may have been renamed while it was
// parked, so the marker is rewritten from the client as it is now.
func RestorePeer(serverConfPath string, client *api.Client) error {
	parked := filepath.Join(SuspendedDir(serverConfPath), client.ID+".conf")
	suspended, err := os.ReadFile(parked)
	if err != nil {
		return fmt.Errorf("reading the suspended peer of %s: %w", client.ID, err)
	}

	block := RetagPeerBlock(strings.Split(strings.TrimRight(string(suspended), "\n"), "\n"), client)
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
