// Package config reads the environment the backend is started with. It is the
// only place that looks at os.Getenv: everything else takes a Settings value
// and can be tested with one built by hand.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Where the backend keeps its state. Fixed paths: the container mounts a
// volume at ConfigDir, and the awg-quick wrappers look for interface .conf
// files under WireguardConfigDir by name.
const (
	ConfigDir          = "/etc/amnezia"
	WireguardConfigDir = "/etc/amnezia/amneziawg"
	ConfigFile         = "/etc/amnezia/web_config.json"
	LogDir             = "/var/log/amnezia"
)

// Settings are the environment-driven defaults, resolved once at startup.
type Settings struct {
	// WebUIPort is the TCP port the panel listens on. It is also reserved
	// against server ports: see Manager.CreateServer.
	WebUIPort int

	// AutoStart brings every server marked auto-start up when the backend
	// boots, and is the default for new servers.
	AutoStart bool

	DefaultMTU        int
	DefaultSubnet     string
	DefaultPort       int
	DNSServers        []string
	EnableObfuscation bool

	// SuspendCheckInterval is how often scheduled client suspensions are
	// looked at.
	SuspendCheckInterval time.Duration

	// Pprof mounts /debug/pprof behind the same credentials as the API.
	Pprof bool

	// User and PasswordHash are the basic-auth credentials; the hash is the
	// base64 of the SHA-256 of the password, as fiber's middleware expects.
	User         string
	PasswordHash string

	// ConfigFile is where web_config.json lives. Tests point it at a temp
	// file so they never touch /etc.
	ConfigFile string

	// WireguardConfigDir is where the per-interface .conf files are written.
	WireguardConfigDir string
}

// FromEnv resolves every setting from the environment, falling back to the
// defaults the container documents.
func FromEnv() Settings {
	s := Settings{
		WebUIPort:            atoiDefault(getenv("WEB_UI_PORT", ""), 54845),
		AutoStart:            strings.EqualFold(getenv("AUTO_START_SERVERS", "true"), "true"),
		DefaultMTU:           atoiDefault(getenv("DEFAULT_MTU", ""), 1280),
		DefaultSubnet:        getenv("DEFAULT_SUBNET", "10.0.0.0/24"),
		DefaultPort:          atoiDefault(getenv("DEFAULT_PORT", ""), 54844),
		DNSServers:           splitList(getenv("DEFAULT_DNS", "8.8.8.8,1.1.1.1")),
		EnableObfuscation:    true,
		SuspendCheckInterval: time.Minute,
		Pprof:                parseBool(os.Getenv("WEB_UI_PPROF")),
		User:                 getenv("WEB_UI_USER", "admin"),
		// SHA-256 hash of "changeme" in base64
		PasswordHash:       getenv("WEB_UI_PASSWORD", "BXugPWxEEEhj3HNh/kV4ll0YhzYPkKCJWILlimJI/IY="),
		ConfigFile:         ConfigFile,
		WireguardConfigDir: WireguardConfigDir,
	}
	return s
}

// EnsureDirectories creates the directories the backend writes into. Errors
// are ignored on purpose: a missing directory surfaces on the first write,
// with a message that names the file.
func (s Settings) EnsureDirectories() {
	os.MkdirAll(ConfigDir, 0o755)
	os.MkdirAll(s.WireguardConfigDir, 0o755)
	os.MkdirAll(LogDir, 0o755)
}

func getenv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// parseBool reads a Go boolean ("1", "t", "true", "TRUE" and their
// negatives); anything else counts as "off" rather than aborting the start.
func parseBool(s string) bool {
	enabled, err := strconv.ParseBool(s)
	return err == nil && enabled
}

// splitList parses a comma separated list, dropping blanks.
func splitList(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
