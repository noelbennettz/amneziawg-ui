package e2e

import (
	"testing"
	"time"

	"amneziawg-web-ui/web-ui/api"
)

// TestNarrowLayout is the phone case: the same page in a 390-point-wide touch
// viewport, where the two-column layout of the desktop does not fit.
//
// It checks the three things a narrow page has to get right - nothing sticks
// out past the right edge, the form folds into one column, and a field can
// still be typed into - and it drives the page by touch rather than by mouse,
// because a phone has no mouse to move first.
func TestNarrowLayout(t *testing.T) {
	servers := getJSON[[]api.Server](t, "/api/servers")
	if len(servers) == 0 {
		t.Fatal("TestCreateServer must run first")
	}
	server := servers[0]

	a := newNarrowApp(t, narrowWidth, 844)
	a.login()
	a.start()

	// Every widget the page offers has to sit inside the viewport. The three
	// here are the ones that used to hang over the edge: the panel's
	// disclosure sits at the right end of its row, the server's menu at the
	// right end of the card header, and the card's own action just under it.
	for _, id := range []string{
		"monitoring.toggle",
		"server." + server.ID + ".menu",
		"server." + server.ID + ".addClient",
	} {
		rect, ok := a.rect(id)
		if !ok {
			t.Fatalf("no widget %q on the narrow page", id)
		}
		if right := rect["x"] + rect["w"]; right > narrowWidth+1 {
			t.Errorf("%s reaches %.0f, past the %d-point viewport: %v", id, right, narrowWidth, rect)
		}
		if rect["x"] < -1 {
			t.Errorf("%s starts at %.0f, off the left edge: %v", id, rect["x"], rect)
		}
	}

	// The create form is one column on a phone: the port field sits under the
	// name field rather than beside it.
	a.tap("form.toggle")
	time.Sleep(700 * time.Millisecond)
	a.shot("12-narrow-form")

	name, ok := a.rect("form.name")
	if !ok {
		t.Fatal("no name field on the narrow form")
	}
	port, ok := a.rect("form.port")
	if !ok {
		t.Fatal("no port field on the narrow form")
	}
	if port["y"] <= name["y"]+name["h"]/2 {
		t.Errorf("port field is at y=%.0f and the name field at y=%.0f: the form is still two columns",
			port["y"], name["y"])
	}

	create, ok := a.rect("form.create")
	if !ok {
		t.Fatal("no create button on the narrow form")
	}
	if right := create["x"] + create["w"]; right > narrowWidth+1 {
		t.Errorf("the create button reaches %.0f, past the %d-point viewport", right, narrowWidth)
	}

	// Typing through the touch path: tapping a field raises the browser's
	// keyboard by focusing the page's hidden input, and that input is what a
	// phone types into.
	a.tapAndType("form.name", "phone")
	a.shot("13-narrow-typed")

	focused, err := a.page.Evaluate(`() => document.activeElement ? document.activeElement.id : ''`)
	must(t, err)
	if focused != "dummyEntry" {
		t.Errorf("active element after tapping a field = %v, want the page's keyboard input", focused)
	}

	// The text arrived: the field cannot be read from the page, so create the
	// server and let the backend say what it was called.
	a.tap("form.create")
	eventually(t, 60*time.Second, "the server named on the narrow form was never created", func() bool {
		for _, s := range getJSON[[]api.Server](t, "/api/servers") {
			if s.Name == "phone" {
				return true
			}
		}
		return false
	})
	a.shot("14-narrow-created")
	a.noPageErrors()
}
