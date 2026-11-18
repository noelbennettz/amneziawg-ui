package wgconf

import (
	"os"
	"path/filepath"
	"strings"

	"amneziawg-web-ui/internal/atomicfile"
	"amneziawg-web-ui/web-ui/api"
)

// The .conf files written before IPv6 support carry one Address line and one
// AllowedIPs per peer, both IPv4 only. These two functions add the IPv6 half to
// a file, and do nothing at all when it is already there: the manager calls
// them on every start, so an upgrade never needs a one-shot rewrite of every
// config, and a file that already says what it should is not touched.

// EnsureInterfaceAddress adds address to the [Interface] Address line when the
// file does not carry it yet. Nothing outside the [Interface] section is
// consulted, so a peer block's own lines can never be mistaken for it.
func EnsureInterfaceAddress(path, address string) error {
	lines, err := readLines(path)
	if err != nil {
		return err
	}

	index := interfaceAddressLine(lines)
	if index < 0 {
		return nil
	}
	current := lineValue(lines[index])
	merged := mergeAddressList(current, address)
	if merged == current {
		return nil
	}
	lines[index] = "Address = " + merged
	return writeLines(path, lines)
}

// EnsurePeerAllowedIPs adds whatever of allowedIPs the client's peer block does
// not route yet. A suspended client's block is parked under SuspendedDir and
// goes back into the .conf when it is activated, so that copy is fixed too.
func EnsurePeerAllowedIPs(confPath string, client *api.Client, allowedIPs string) error {
	found, err := ensurePeerAllowedIPsIn(confPath, client, allowedIPs)
	if err != nil || found {
		return err
	}

	parked := filepath.Join(SuspendedDir(confPath), client.ID+".conf")
	if _, statErr := os.Stat(parked); statErr != nil {
		return nil
	}
	_, err = ensurePeerAllowedIPsIn(parked, client, allowedIPs)
	return err
}

// ensurePeerAllowedIPsIn edits one file, reporting whether it holds the
// client's block at all.
func ensurePeerAllowedIPsIn(path string, client *api.Client, allowedIPs string) (found bool, err error) {
	lines, err := readLines(path)
	if err != nil {
		return false, err
	}

	start := -1
	for i, line := range lines {
		if MarkerMatches(line, client) {
			start = i
			break
		}
	}
	if start < 0 {
		return false, nil
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if _, _, isMarker := ParsePeerMarker(lines[i]); isMarker {
			end = i
			break
		}
	}

	for i := start; i < end; i++ {
		key, _, ok := splitKeyValue(lines[i])
		if !ok || key != "AllowedIPs" {
			continue
		}
		merged := mergeAddressList(lineValue(lines[i]), allowedIPs)
		if merged == lineValue(lines[i]) {
			return true, nil
		}
		lines[i] = "AllowedIPs = " + merged
		return true, writeLines(path, lines)
	}
	return true, nil
}

// interfaceAddressLine is the index of the Address line of the [Interface]
// section: the first one before any peer block begins. -1 when the file has
// none.
func interfaceAddressLine(lines []string) int {
	for i, line := range lines {
		if _, _, isMarker := ParsePeerMarker(line); isMarker {
			return -1
		}
		if strings.EqualFold(strings.TrimSpace(line), "[Peer]") {
			return -1
		}
		if key, _, ok := splitKeyValue(line); ok && key == "Address" {
			return i
		}
	}
	return -1
}

// mergeAddressList returns the union of the current and the wanted addresses
// on one comma separated line: order kept, duplicates dropped, so running it
// twice produces the same line.
func mergeAddressList(current, wanted string) string {
	seen := map[string]bool{}
	merged := make([]string, 0, 2)
	for _, list := range []string{current, wanted} {
		for _, entry := range strings.Split(list, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" || seen[entry] {
				continue
			}
			seen[entry] = true
			merged = append(merged, entry)
		}
	}
	return strings.Join(merged, ", ")
}

// splitKeyValue splits a "Key = value" line; ok is false for anything else.
func splitKeyValue(line string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

// lineValue is the value of a "Key = value" line, or "" when it has none.
func lineValue(line string) string {
	_, value, _ := splitKeyValue(line)
	return value
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n"), nil
}

func writeLines(path string, lines []string) error {
	return atomicfile.Write(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}
