// Package wgconf renders and edits the WireGuard .conf files the backend
// writes: the server interface, the peer blocks appended to it, and the
// client configs handed out. It knows the file format and nothing about the
// servers and clients the files describe.
package wgconf

import (
	"fmt"
	"strings"

	"amneziawg-web-ui/web-ui/api"
)

// Peer blocks in a server's .conf are introduced by a comment that names the
// client they belong to, and that comment is how this code finds a block
// again when the client is removed or suspended. The client's name is not an
// identity: two clients may share one, and a name is free text that can
// contain anything the operator typed. So the marker carries the client ID as
// well, and every lookup matches on that.
//
//	# Client: alice [id:ab12cd]
//	[Peer]
//	PublicKey = ...
//
// A comment without an ID - hand-written, or left by something else - still
// delimits a block, so removing the peer above it cannot swallow it, but it
// never matches a client.

const peerMarkerPrefix = "# Client:"

// PeerMarker renders the comment introducing a client's peer block.
func PeerMarker(name, clientID string) string {
	return fmt.Sprintf("%s %s [id:%s]", peerMarkerPrefix, name, clientID)
}

// ParsePeerMarker splits a marker line into the client name and ID. ok is
// false for any other line; id is empty for a marker that carries none.
func ParsePeerMarker(line string) (name, id string, ok bool) {
	trimmed := strings.TrimSpace(line)
	rest, ok := strings.CutPrefix(trimmed, peerMarkerPrefix)
	if !ok {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)

	if strings.HasSuffix(rest, "]") {
		if head, tail, found := strings.Cut(rest[:len(rest)-1], "[id:"); found {
			return strings.TrimSpace(head), strings.TrimSpace(tail), true
		}
	}
	return rest, "", true
}

// MarkerMatches reports whether a marker line introduces this client's block.
// Only the ID decides: a name is neither unique nor trusted input.
func MarkerMatches(line string, client *api.Client) bool {
	_, id, ok := ParsePeerMarker(line)
	return ok && id != "" && id == client.ID
}

// SplitPeerBlock finds the client's peer block and returns the config lines
// with the block removed, plus the block itself. The block runs from its
// marker to the line before the next marker, trailing blank lines excluded,
// so removing it cannot swallow the peer that follows.
func SplitPeerBlock(lines []string, client *api.Client) (rest, block []string, found bool) {
	start := -1
	for i, line := range lines {
		if MarkerMatches(line, client) {
			start = i
			break
		}
	}
	if start < 0 {
		return lines, nil, false
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if _, _, isMarker := ParsePeerMarker(lines[i]); isMarker {
			end = i
			break
		}
	}

	block = trimTrailingBlank(lines[start:end])
	rest = append(append([]string{}, lines[:start]...), lines[end:]...)
	return trimTrailingBlank(rest), block, true
}

func trimTrailingBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// RetagPeerBlock rewrites a stored block's marker from the client as it is
// now, so a client renamed while suspended comes back under its current name.
func RetagPeerBlock(block []string, client *api.Client) []string {
	out := append([]string{}, block...)
	for i, line := range out {
		if _, _, ok := ParsePeerMarker(line); ok {
			out[i] = PeerMarker(client.Name, client.ID)
			break
		}
	}
	return out
}

// PeerBlock renders the block a server's .conf gets for one client, marker
// included, with a leading blank line so it never runs into the previous
// section.
func PeerBlock(client *api.Client, allowedIPs string) string {
	return fmt.Sprintf("\n%s\n[Peer]\nPublicKey = %s\nPresharedKey = %s\nAllowedIPs = %s\n",
		PeerMarker(client.Name, client.ID), client.ClientPublicKey, client.PresharedKey, allowedIPs)
}
