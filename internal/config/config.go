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
	// AuditFile is where the security log is appended when it is switched on.
	// It lives on the same volume as the config, so it survives a restart of
	// the container and is the operator's to read.
	AuditFile = "/etc/amnezia/audit.log"
	LogDir    = "/var/log/amnezia"
)

// SecureMode is how the Secure flag on the cookies is decided.
type SecureMode int

const (
	// SecureAuto marks a cookie Secure when the request itself arrived over
	// TLS, or when a proxy in front of the panel said it did.
	SecureAuto SecureMode = iota
	// SecureOn marks every cookie Secure. Behind a TLS proxy the container
	// sees plain HTTP, so this is the setting that matches reality.
	SecureOn
	// SecureOff never marks a cookie Secure, for a deployment on plain HTTP
	// where a Secure cookie would be dropped by the browser.
	SecureOff
)

// Settings are the environment-driven defaults, resolved once at startup.
type Settings struct {
	// WebUIPort is the TCP port the panel listens on. It is also reserved
	// against server ports: see Manager.CreateServer.
	WebUIPort int

	// WebUIBind is the address the panel binds. Empty means every address on
	// both stacks; "127.0.0.1" is what a deployment behind a reverse proxy
	// and a published port wants.
	WebUIBind string

	// SecureCookie is the policy for the Secure flag on the panel's cookies.
	SecureCookie SecureMode

	// AuditLog appends login, logout and every change made through the API
	// to AuditFile. Off by default: it writes personal data (addresses) to
	// the config volume, so it is the operator's decision.
	AuditLog bool

	// AuditRetention is how long an entry stays in that file. Default a day,
	// ceiling a month, adjustable with WEB_UI_AUDIT_RETENTION_HOURS.
	AuditRetention time.Duration

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

	// User and PasswordHash are the credentials the login page checks; the
	// hash is the base64 of the SHA-256 of the password, the format the basic
	// auth header still being accepted for scripts uses too.
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
		WebUIBind:            getenv("WEB_UI_BIND", ""),
		SecureCookie:         secureMode(os.Getenv("WEB_UI_SECURE_COOKIE")),
		AuditLog:             parseBool(os.Getenv("WEB_UI_AUDIT_LOG")),
		AuditRetention:       auditRetention(os.Getenv("WEB_UI_AUDIT_RETENTION_HOURS")),
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

// secureMode reads WEB_UI_SECURE_COOKIE: an explicit true or false pins the
// Secure flag, anything else - unset above all - leaves the decision to the
// request.
func secureMode(s string) SecureMode {
	enabled, err := strconv.ParseBool(s)
	switch {
	case err != nil:
		return SecureAuto
	case enabled:
		return SecureOn
	default:
		return SecureOff
	}
}

// auditRetention reads WEB_UI_AUDIT_RETENTION_HOURS. A day by default, at least
// an hour, at most a month: the log is a record of who reached the panel, and a
// record that never ends is a liability.
func auditRetention(s string) time.Duration {
	const defaultHours, maxHours = 24, 720

	hours := atoiDefault(s, defaultHours)
	switch {
	case hours < 1:
		hours = 1
	case hours > maxHours:
		hours = maxHours
	}
	return time.Duration(hours) * time.Hour
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
