package manager

import (
	"encoding/base64"
	"errors"
	"testing"

	"amneziawg-web-ui/internal/wgconf"
	"amneziawg-web-ui/web-ui/api"
)

// The shape of a generated parameter set is the shared generator's business
// and is tested there. What belongs here are the two fields it deliberately
// leaves alone, because only the backend can fill them: the MTU it was asked
// for, and a key from a cryptographic source.
func TestGeneratedObfuscationParamsCarryTheMTUAndAKey(t *testing.T) {
	for _, mtu := range []int{api.MinMTU, 1420, api.MaxMTU} {
		p := generateObfuscationParams(mtu)

		if p.MTU != mtu {
			t.Errorf("MTU = %d, want %d", p.MTU, mtu)
		}
		key, err := base64.StdEncoding.DecodeString(p.HeaderProtectionKey)
		if err != nil {
			t.Errorf("MTU %d: header protection key %q is not base64: %v", mtu, p.HeaderProtectionKey, err)
			continue
		}
		if len(key) != 32 {
			t.Errorf("MTU %d: header protection key is %d bytes, want 32", mtu, len(key))
		}
		if err := validateObfuscationParams(&p, mtu); err != nil {
			t.Errorf("MTU %d: generated params rejected: %v", mtu, err)
		}
		if !p.RandomTrailers || !p.DisableCookies {
			t.Errorf("generated params should enable the 3.1 switches: %+v", p)
		}
	}
}

// Two servers must not come out with the same header protection key.
func TestGeneratedHeaderProtectionKeysDiffer(t *testing.T) {
	first := generateObfuscationParams(api.MinMTU).HeaderProtectionKey
	second := generateObfuscationParams(api.MinMTU).HeaderProtectionKey
	if first == second {
		t.Fatalf("both servers got the same key %q", first)
	}
}

// validateObfuscationParams is a thin wrapper: the rules are the shared ones,
// and its own job is to turn them into one error the handlers can map to a
// 400 - including the MTU-dependent checks, which it used to skip entirely.
func TestValidateObfuscationParamsReportsSharedRules(t *testing.T) {
	p := api.ObfuscationParams{
		Jc: 8, Jmin: 8, Jmax: 80,
		S1: 50, S2: 50, S3: 50, S4: 50,
		H1: 1, H2: 2, H3: 3, H4: 4,
	}
	if err := validateObfuscationParams(&p, api.MinMTU); err != nil {
		t.Fatalf("valid parameters rejected: %v", err)
	}

	p.S1 = api.MinMTU - 147 // one past MTU-148
	if err := validateObfuscationParams(&p, api.MinMTU); err == nil {
		t.Fatal("want the MTU-dependent rule to be enforced, got no error")
	}

	if err := validateObfuscationParams(nil, api.MinMTU); err == nil {
		t.Fatal("want an error for a missing parameter set, got none")
	}
}

// The set a new client starts from has to satisfy the rules the app enforces
// on everyone else.
func TestDefaultISettingsAreValid(t *testing.T) {
	if err := validateISettings(wgconf.DefaultISettings()); err != nil {
		t.Fatalf("the built-in defaults do not validate: %v", err)
	}
}

// A malformed signature packet is a bad request, not a server error, and it is
// rejected before any client is created.
func TestAddClientRejectsAMalformedSignaturePacket(t *testing.T) {
	m, _ := newTestManager(t)

	_, _, err := m.AddClient("s1", api.AddClientRequest{Name: "alice", ApplyISettings: true, ISettings: map[string]string{"i1": "<b 0xabc>"}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	if got := m.ClientCount(); got != 0 {
		t.Errorf("nothing should have been created, got %d clients", got)
	}
}
