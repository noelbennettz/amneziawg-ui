package auth

import (
	"io"
	"strings"
	"testing"
)

// The source link on the login page is the AGPL section 13 offer, and the login
// page is the only thing this panel serves before a session exists: following
// the link in the same tab replaces the form, and coming back means logging in
// again. It has to open in a new tab, and that tab must not get a handle on the
// page it came from.
func TestLoginPageSourceLinkOpensANewTab(t *testing.T) {
	app := newTestApp(t)

	resp := get(t, app, LoginPath, nil, nil)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the login page: %v", err)
	}
	page := string(body)

	at := strings.Index(page, "github.com/noelbennettz/amneziawg-ui")
	if at < 0 {
		t.Fatal("the login page has no source link")
	}
	start := strings.LastIndex(page[:at], "<a ")
	if start < 0 {
		t.Fatalf("the source URL is not inside an anchor:\n%s", page)
	}
	end := strings.Index(page[start:], ">")
	if end < 0 {
		t.Fatalf("the source anchor has no closing bracket:\n%s", page)
	}
	tag := page[start : start+end+1]

	if !strings.Contains(tag, `target="_blank"`) {
		t.Errorf("the source link does not open in a new tab: %s", tag)
	}
	if !strings.Contains(tag, "noopener") {
		t.Errorf("the source link does not carry rel=noopener: %s", tag)
	}
}
