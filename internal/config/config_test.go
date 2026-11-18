package config

import (
	"slices"
	"testing"
	"time"
)

func TestFromEnvReadsTheDocumentedVariables(t *testing.T) {
	t.Setenv("WEB_UI_PORT", "8080")
	t.Setenv("AUTO_START_SERVERS", "FALSE")
	t.Setenv("DEFAULT_MTU", "1420")
	t.Setenv("DEFAULT_DNS", " 9.9.9.9 ,, 1.0.0.1")
	t.Setenv("WEB_UI_PPROF", "1")

	s := FromEnv()
	if s.WebUIPort != 8080 || s.AutoStart || s.DefaultMTU != 1420 || !s.Pprof {
		t.Errorf("Settings = %+v", s)
	}
	if !slices.Equal(s.DNSServers, []string{"9.9.9.9", "1.0.0.1"}) {
		t.Errorf("DNSServers = %v", s.DNSServers)
	}
}

func TestFromEnvFallsBackOnGarbage(t *testing.T) {
	t.Setenv("WEB_UI_PORT", "eighty")
	t.Setenv("WEB_UI_PPROF", "yes please")

	s := FromEnv()
	if s.WebUIPort != 54845 || s.Pprof {
		t.Errorf("Settings = %+v", s)
	}
}

// The three security settings are off unless asked for: the panel binds every
// address, leaves the Secure flag to the request, and keeps no audit log.
func TestSecuritySettingsDefaultToOff(t *testing.T) {
	s := FromEnv()

	if s.WebUIBind != "" {
		t.Errorf("WebUIBind = %q, want empty (every address)", s.WebUIBind)
	}
	if s.SecureCookie != SecureAuto {
		t.Errorf("SecureCookie = %v, want auto", s.SecureCookie)
	}
	if s.AuditLog {
		t.Error("AuditLog is on by default")
	}
	if s.LoginRateLimit {
		t.Error("LoginRateLimit is on by default")
	}
	if s.TrustProxy {
		t.Error("TrustProxy is on by default")
	}
}

// The audit log keeps a day of events by default, and the operator can widen
// that to a month - no further.
func TestAuditRetention(t *testing.T) {
	if got := FromEnv().AuditRetention; got != 24*time.Hour {
		t.Errorf("default retention = %v, want 24h", got)
	}

	for value, want := range map[string]time.Duration{
		"6":    6 * time.Hour,
		"720":  720 * time.Hour,
		"0":    time.Hour,       // under the floor
		"-5":   time.Hour,       // under the floor
		"9999": 720 * time.Hour, // over the ceiling
		"soon": 24 * time.Hour,  // not a number
	} {
		t.Setenv("WEB_UI_AUDIT_RETENTION_HOURS", value)
		if got := FromEnv().AuditRetention; got != want {
			t.Errorf("%s: retention = %v, want %v", value, got, want)
		}
	}
}

func TestSecuritySettingsFromTheEnvironment(t *testing.T) {
	t.Setenv("WEB_UI_BIND", "127.0.0.1")
	t.Setenv("WEB_UI_SECURE_COOKIE", "true")
	t.Setenv("WEB_UI_AUDIT_LOG", "1")
	t.Setenv("WEB_UI_LOGIN_RATE_LIMIT", "true")
	t.Setenv("WEB_UI_TRUST_PROXY", "true")

	s := FromEnv()
	if s.WebUIBind != "127.0.0.1" || s.SecureCookie != SecureOn || !s.AuditLog || !s.LoginRateLimit || !s.TrustProxy {
		t.Errorf("Settings = %+v", s)
	}

	// An explicit false is not the same as unset: it pins the flag off even
	// when a proxy reports https.
	t.Setenv("WEB_UI_SECURE_COOKIE", "false")
	if got := FromEnv().SecureCookie; got != SecureOff {
		t.Errorf("SecureCookie = %v, want off", got)
	}

	// Anything that is not a boolean leaves the decision to the request.
	t.Setenv("WEB_UI_SECURE_COOKIE", "sometimes")
	if got := FromEnv().SecureCookie; got != SecureAuto {
		t.Errorf("SecureCookie = %v, want auto", got)
	}
}
