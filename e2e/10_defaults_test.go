package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"amneziawg-web-ui/web-ui/api"
)

// TestAutoStartFollowsTheEnvironment drives the second instance `make e2e`
// starts with AUTO_START_SERVERS=false. The create form's "auto-start" switch
// takes its initial value from that setting, and a server created without
// touching the switch has to come out stopped - which is the only way to see
// the switch's state from outside, since the panel is a canvas.
//
// A checkout that does not run the second instance skips this rather than
// failing: the instance is the suite's own, not the operator's.
func TestAutoStartFollowsTheEnvironment(t *testing.T) {
	base := strings.TrimRight(os.Getenv("AWG_URL_NOAUTOSTART"), "/")
	if base == "" {
		t.Skip("AWG_URL_NOAUTOSTART is not set: `make e2e` starts the instance it names")
	}

	a := newAppAt(t, base)
	a.loginAt(base)
	a.start()

	status := getJSONAt[api.SystemStatus](t, base, "/api/system/status")
	if status.Environment.AutoStartServers {
		t.Fatal("this instance reports auto-start on, but it was started with AUTO_START_SERVERS=false")
	}

	a.tap("form.toggle")
	a.tapAndType("form.name", "No Auto Start")
	a.tap("form.create")

	eventually(t, 60*time.Second, "the server never showed up in /api/servers", func() bool {
		return len(getJSONAt[[]api.Server](t, base, "/api/servers")) == 1
	})

	server := getJSONAt[[]api.Server](t, base, "/api/servers")[0]
	if server.Status != "stopped" {
		t.Errorf("status = %q, want stopped: the form's auto-start switch follows AUTO_START_SERVERS", server.Status)
	}
	if server.MTU != status.Environment.DefaultMTU {
		t.Errorf("MTU = %d, want DEFAULT_MTU %d", server.MTU, status.Environment.DefaultMTU)
	}
	a.shot("10-no-autostart")
}
