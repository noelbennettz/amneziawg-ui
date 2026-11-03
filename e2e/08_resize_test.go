package e2e

import (
	"testing"
	"time"

	"amneziawg-web-ui/web-ui/api"
)

// TestWindowResizeRefolds shrinks a desktop window without reloading the page.
//
// This is the case a user hits by dragging the browser window narrow: fyne
// grows its canvas to at least the content's minimum, so a page that reports
// the 700 points of its two-column layout gets laid out at 700 inside a
// narrower window and can never see that the window is narrow - the cards to
// the right are simply clipped away. The page now folds to the compact layout
// and stays inside the window, in both directions.
func TestWindowResizeRefolds(t *testing.T) {
	servers := getJSON[[]api.Server](t, "/api/servers")
	if len(servers) == 0 {
		t.Fatal("TestCreateServer must run first")
	}
	server := servers[0]

	a := startApp(t)

	wide, ok := a.rect("form.toggle")
	if !ok {
		t.Fatal("no create toggle on the wide page")
	}
	if wide["w"] < 600 {
		t.Fatalf("the create toggle is %.0f wide at %d points: not the two-column layout",
			wide["w"], viewportWidth)
	}

	// Drag the window down to a phone-ish width, no reload.
	must(t, a.page.SetViewportSize(narrowWidth, 684))
	time.Sleep(2 * time.Second)
	a.shot("15-resized-narrow")

	for _, id := range []string{
		"monitoring.toggle",
		"server." + server.ID + ".menu",
		"server." + server.ID + ".addClient",
		"form.toggle",
	} {
		rect, ok := a.rect(id)
		if !ok {
			t.Fatalf("no widget %q after the window shrank", id)
		}
		if right := rect["x"] + rect["w"]; right > narrowWidth+1 {
			t.Errorf("%s reaches %.0f after the resize, past the %d-point window: %v",
				id, right, narrowWidth, rect)
		}
	}

	// The form is one column again, which is what the fold is measured by.
	a.tap("form.toggle")
	time.Sleep(700 * time.Millisecond)
	name, ok := a.rect("form.name")
	if !ok {
		t.Fatal("no name field after the window shrank")
	}
	port, ok := a.rect("form.port")
	if !ok {
		t.Fatal("no port field after the window shrank")
	}
	if port["y"] <= name["y"]+name["h"]/2 {
		t.Errorf("port field is at y=%.0f and the name field at y=%.0f: the form is still two columns",
			port["y"], name["y"])
	}
	a.shot("16-resized-form")
	a.noPageErrors()
}
