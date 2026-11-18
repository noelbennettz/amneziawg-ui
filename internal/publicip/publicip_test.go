package publicip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// answer stands in for one detection service: it replies with body, which is
// all the real ones do. The package-level lists are swapped for the test and
// restored afterwards, so nothing here reaches the network.
func answer(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func stub(t *testing.T, v4, v6 []string) {
	t.Helper()
	oldV4, oldV6 := Services, Services6
	Services, Services6 = v4, v6
	t.Cleanup(func() { Services, Services6 = oldV4, oldV6 })
}

func static(ip string, err error) func() (string, error) {
	return func() (string, error) { return ip, err }
}

// A dual-stack host keeps the IPv4 answer: it is the address every client can
// still reach, so detection must not start preferring IPv6 just because one
// is on offer.
func TestDetectPrefersIPv4(t *testing.T) {
	stub(t, []string{answer(t, "203.0.113.5\n")}, []string{answer(t, "2001:db8::1\n")})

	if got := Detect(nil, nil); got != "203.0.113.5" {
		t.Fatalf("Detect = %q, want the IPv4 answer", got)
	}
}

// A v6-only host used to get Unknown here, and every config generated from
// it pointed at a host that does not exist.
func TestDetectFallsBackToIPv6(t *testing.T) {
	// The v4 list answers with an IPv6 address - what ifconfig.me does for a
	// v6 caller - and must not be taken for the IPv4 address.
	stub(t, []string{answer(t, "2001:db8::1\n")}, []string{answer(t, "2001:db8::2\n")})

	if got := Detect(nil, nil); got != "2001:db8::2" {
		t.Fatalf("Detect = %q, want the IPv6 answer", got)
	}
}

// With no service reachable the routing tables get a say, v4 first and v6
// second.
func TestDetectFallsBackToTheRoutingTables(t *testing.T) {
	stub(t, nil, nil)

	if got := Detect(static("203.0.113.5", nil), static("2001:db8::1", nil)); got != "203.0.113.5" {
		t.Fatalf("Detect = %q, want the IPv4 route source", got)
	}
	if got := Detect(static("2001:db8::1", nil), static("2001:db8::2", nil)); got != "2001:db8::2" {
		t.Fatalf("Detect = %q, want the IPv6 route source", got)
	}
	// A route source of the wrong family, or a failing command, is no answer.
	if got := Detect(static("garbage", nil), static("", nil)); got != Unknown {
		t.Fatalf("Detect = %q, want %q", got, Unknown)
	}
	if got := Detect(nil, nil); got != Unknown {
		t.Fatalf("Detect with nothing at all = %q, want %q", got, Unknown)
	}
}

// A service that answers with something that is not an address is skipped,
// not handed to clients.
func TestDetectIgnoresJunk(t *testing.T) {
	stub(t, []string{answer(t, "<html>nope</html>")}, []string{answer(t, "::1\n")})

	if got := Detect(nil, nil); got != "::1" {
		t.Fatalf("Detect = %q, want ::1", got)
	}
}
