package e2e

import (
	"testing"
	"time"

	"amneziawg-web-ui/web-ui/api"
)

// TestDialogFitsOnAPhone opens the client's config page - a QR code above a
// column of config - in a phone-sized window.
//
// It is the tallest dialog the page has, and it used to grow past the screen
// and take its only button with it. The card now keeps the room it is given,
// scrolls what does not fit and pins the action row, so the close button has to
// be inside the viewport.
func TestDialogFitsOnAPhone(t *testing.T) {
	servers := getJSON[[]api.Server](t, "/api/servers")
	if len(servers) == 0 {
		t.Fatal("TestCreateServer must run first")
	}
	clients := getJSON[[]api.Client](t, "/api/servers/"+servers[0].ID+"/clients")
	if len(clients) == 0 {
		t.Fatal("TestAddClient must run first")
	}

	const height = 844
	a := newNarrowApp(t, narrowWidth, height)
	a.login()
	a.start()

	a.act("client." + clients[0].ID + ".menu.qr")
	time.Sleep(2 * time.Second)
	a.shot("17-phone-qr")

	for _, id := range []string{"dialog.close"} {
		rect, ok := a.rect(id)
		if !ok {
			t.Fatalf("no %q on the config page", id)
		}
		if rect["y"] < -1 || rect["y"]+rect["h"] > height+1 {
			t.Errorf("%s is at y=%.0f..%.0f, outside the %d-point window: %v",
				id, rect["y"], rect["y"]+rect["h"], height, rect)
		}
		if rect["x"] < -1 || rect["x"]+rect["w"] > narrowWidth+1 {
			t.Errorf("%s is at x=%.0f..%.0f, outside the %d-point window: %v",
				id, rect["x"], rect["x"]+rect["w"], narrowWidth, rect)
		}
	}
	a.noPageErrors()
}
