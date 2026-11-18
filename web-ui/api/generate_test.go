package api

import (
	"strings"
	"testing"
)

// Both sides generate through this: the create form's simple mode sends what
// it produces without ever showing the user a field to fix, and the backend
// falls back to it for an API caller that sends no parameters. So whatever it
// rolls has to already satisfy Validate, at any MTU a server may have and on
// every roll - a set that is invalid one time in a thousand surfaces as a
// create that fails for no visible reason.
func TestGenerateObfuscationAlwaysValidates(t *testing.T) {
	for _, mtu := range []int{MinMTU, 1330, 1420, MaxMTU} {
		for _, distinct := range []bool{false, true} {
			for range 500 {
				p := GenerateObfuscation(mtu, distinct)

				if problems := p.Validate(mtu); len(problems) > 0 {
					t.Fatalf("MTU %d, distinct %v: generated %+v, rejected by its own validation:\n%s",
						mtu, distinct, p, strings.Join(problems, "\n"))
				}
				if !p.RandomTrailers || !p.DisableCookies {
					t.Fatalf("MTU %d: the 3.1 switches should default to on, got %+v", mtu, p)
				}
				if p.H1 != 1 || p.H2 != 2 || p.H3 != 3 || p.H4 != 4 {
					t.Fatalf("MTU %d: headers should stay at the standard types, got %d/%d/%d/%d",
						mtu, p.H1, p.H2, p.H3, p.H4)
				}
				// The key is the backend's to generate, from a cryptographic
				// source; the generator must not hand one out.
				if p.HeaderProtectionKey != "" {
					t.Fatalf("MTU %d: generated a header protection key %q", mtu, p.HeaderProtectionKey)
				}
				// A full transport packet - tunnel MTU plus the WireGuard
				// transport overhead plus the padding plus IPv4/UDP - should
				// stay under a standard 1500-byte path, or every one of them
				// is sent as fragments. Past a tunnel MTU of about 1425 that
				// is not reachable at all, and all the generator can do is not
				// make it worse than the floor.
				if wire := mtu + TransportPacketBytes + p.S4; wire > AssumedPathMTU && p.S4 != MinRecommendedPadding {
					t.Fatalf("MTU %d: a full transport packet would be %d bytes on the wire (S4 %d)", mtu, wire, p.S4)
				}
			}
		}
	}
}

// The default shape is the one the AmneziaWG docs recommend for header
// protection with random trailers: one padding for all four message types.
func TestGenerateObfuscationRepeatsOnePadding(t *testing.T) {
	for range 100 {
		p := GenerateObfuscation(MinMTU, false)
		if !(p.S1 == p.S2 && p.S2 == p.S3 && p.S3 == p.S4) {
			t.Fatalf("want all four paddings equal, got %d/%d/%d/%d", p.S1, p.S2, p.S3, p.S4)
		}
	}
}

// Asking for separate values has to actually produce them often enough to be
// worth the switch - a generator that happened to repeat itself would be a
// silent no-op.
func TestGenerateObfuscationCanVaryPaddings(t *testing.T) {
	varied := 0
	for range 100 {
		p := GenerateObfuscation(MinMTU, true)
		if !(p.S1 == p.S2 && p.S2 == p.S3 && p.S3 == p.S4) {
			varied++
		}
	}
	if varied == 0 {
		t.Fatal("100 distinct rolls produced four equal paddings every time")
	}
}

// At an MTU that leaves no room the window collapses onto its floor, and
// "separately" cannot mean anything - the four still have to be legal.
func TestPaddingsCollapseOnACrampedMTU(t *testing.T) {
	s1, s2, s3, s4 := RecommendedPaddings(MaxMTU, true)

	if s1 != MinRecommendedPadding || s2 != MinRecommendedPadding ||
		s3 != MinRecommendedPadding || s4 != MinRecommendedPadding {
		t.Fatalf("want all four at the floor %d, got %d/%d/%d/%d", MinRecommendedPadding, s1, s2, s3, s4)
	}
}
