package e2e

import (
	"net"
	"testing"
	"time"

	"amneziawg-web-ui/web-ui/api"
)

func TestAddClient(t *testing.T) {
	a := startApp(t)

	servers := getJSON[[]api.Server](t, "/api/servers")
	if len(servers) == 0 {
		t.Fatal("TestCreateServer must run first")
	}
	server := servers[0]
	clientsPath := "/api/servers/" + server.ID + "/clients"

	// "Add client" on the server card.
	a.tap("server." + server.ID + ".addClient")
	time.Sleep(1200 * time.Millisecond)
	a.shot("06-client-dialog")

	a.tapAndType("dialog.client.name", "laptop")
	a.tap("dialog.confirm")

	eventually(t, 60*time.Second, "the client never showed up in "+clientsPath, func() bool {
		return len(getJSON[[]api.Client](t, clientsPath)) == 1
	})

	client := getJSON[[]api.Client](t, clientsPath)[0]
	if client.Name != "laptop" {
		t.Errorf("name = %q, want %q", client.Name, "laptop")
	}
	// The client's address comes from the server's own subnet, whatever
	// DEFAULT_SUBNET the instance was started with.
	_, network, err := net.ParseCIDR(server.Subnet)
	if err != nil {
		t.Fatalf("server subnet %q: %v", server.Subnet, err)
	}
	if !network.Contains(net.ParseIP(client.ClientIP)) {
		t.Errorf("client_ip = %q, want one in %s", client.ClientIP, server.Subnet)
	}

	time.Sleep(4 * time.Second)
	a.shot("07-client-added")
	a.noPageErrors()
}
