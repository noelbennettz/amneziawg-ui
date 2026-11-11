package security

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func newApp(nonce string) *fiber.App {
	app := fiber.New()
	app.Use(Headers(nonce))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("panel") })
	return app
}

// Every response carries the four headers, the login page and the redirects
// included - they are set before anything that could answer.
func TestHeadersAreOnEveryResponse(t *testing.T) {
	app := newApp("bm9uY2U")

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}

	for header, want := range map[string]string{
		fiber.HeaderXFrameOptions:       "DENY",
		fiber.HeaderXContentTypeOptions: "nosniff",
		fiber.HeaderReferrerPolicy:      "no-referrer",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if got := resp.Header.Get(fiber.HeaderContentSecurityPolicy); got == "" {
		t.Error("no Content-Security-Policy")
	}
}

// The policy names the nonce when there is one, and falls back to allowing
// inline scripts rather than breaking a page it cannot see into.
func TestContentSecurityPolicy(t *testing.T) {
	withNonce := ContentSecurityPolicy("bm9uY2U")
	if !strings.Contains(withNonce, "'nonce-bm9uY2U'") {
		t.Errorf("the nonce is not in the policy: %s", withNonce)
	}
	if scriptSrc := directive(withNonce, "script-src"); strings.Contains(scriptSrc, "'unsafe-inline'") {
		t.Errorf("a nonce is offered and 'unsafe-inline' with it: %s", scriptSrc)
	}

	fallback := ContentSecurityPolicy("")
	if strings.Contains(fallback, "'nonce-") {
		t.Errorf("an empty nonce is still offered as one: %s", fallback)
	}
	if scriptSrc := directive(fallback, "script-src"); !strings.Contains(scriptSrc, "'unsafe-inline'") {
		t.Errorf("the fallback does not allow the page's own script: %s", scriptSrc)
	}

	for _, want := range []string{
		"default-src 'self'",
		"object-src 'none'",
		"base-uri 'none'",
		"frame-ancestors 'none'",
		"form-action 'self'",
		"connect-src 'self'",
	} {
		if !strings.Contains(withNonce, want) {
			t.Errorf("the policy does not contain %q: %s", want, withNonce)
		}
	}
}

// A nonce that can be guessed is not a nonce.
func TestNewNonceIsNotRepeated(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		nonce := NewNonce()
		if len(nonce) < 16 {
			t.Fatalf("nonce %q is too short", nonce)
		}
		if seen[nonce] {
			t.Fatalf("nonce %q came back twice", nonce)
		}
		seen[nonce] = true

	}
}

// directive digs one directive out of a policy string.
func directive(policy, name string) string {
	for part := range strings.SplitSeq(policy, ";") {
		if trimmed := strings.TrimSpace(part); strings.HasPrefix(trimmed, name+" ") {
			return trimmed
		}
	}
	return ""
}
