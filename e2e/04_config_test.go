package e2e

import (
	"strings"
	"testing"
	"time"

	"amneziawg-web-ui/web-ui/api"
)

func TestClientConfig(t *testing.T) {
	a := startApp(t)

	servers := getJSON[[]api.Server](t, "/api/servers")
	if len(servers) == 0 {
		t.Fatal("TestCreateServer must run first")
	}
	server := servers[0]
	clients := getJSON[[]api.Client](t, "/api/servers/"+server.ID+"/clients")
	if len(clients) == 0 {
		t.Fatal("TestAddClient must run first")
	}
	clientPath := "/api/servers/" + server.ID + "/clients/" + clients[0].ID

	// The backend has to offer both views the dialog renders, and the download
	// has to carry the full config with the obfuscation parameters.
	configs := getJSON[api.ClientConfigs](t, clientPath+"/config-both")
	if !strings.Contains(configs.CleanConfig, "[Interface]") {
		t.Errorf("clean_config has no [Interface]:\n%s", configs.CleanConfig)
	}
	// Both stacks: the client is handed an IPv6 address and routes it into the
	// tunnel, so the server has to know it as well.
	if clients[0].ClientIPV6 == "" {
		t.Error("the client has no IPv6 address")
	}
	if !strings.Contains(configs.CleanConfig, clients[0].ClientIPV6+"/128") {
		t.Errorf("clean_config lacks the client's IPv6 address %s:\n%s", clients[0].ClientIPV6, configs.CleanConfig)
	}
	serverConfig := getJSON[api.ServerConfig](t, "/api/servers/"+server.ID+"/config")
	if !strings.Contains(serverConfig.ConfigContent, clients[0].ClientIPV6+"/128") {
		t.Errorf("the server does not route the client's IPv6 address:\n%s", serverConfig.ConfigContent)
	}
	if !strings.Contains(serverConfig.ConfigContent, server.ServerIPV6+"/") {
		t.Errorf("the server has no IPv6 address on its interface:\n%s", serverConfig.ConfigContent)
	}
	link := getJSON[api.AmneziaLink](t, clientPath+"/link")
	if !strings.HasPrefix(link.VPNURL, "vpn://") {
		t.Errorf("vpn_url = %q, want a vpn:// link", link.VPNURL)
	}
	if download := string(apiGet(t, clientPath+"/config")); !strings.Contains(download, "Jc =") {
		t.Errorf("the downloaded config has no Jc:\n%s", download)
	}

	// "QR / config" on the client row. The dialog opens on the .conf tab,
	// which is the one that gets a QR code.
	a.act("client." + clients[0].ID + ".menu.qr")
	time.Sleep(2500 * time.Millisecond)
	a.shot("08-qr-conf")

	// Switch to the AmneziaVPN link tab.
	a.act("config.tab.1")
	time.Sleep(1500 * time.Millisecond)
	a.shot("09-qr-link")

	// Close, then open the server configuration dialog.
	a.tap("dialog.close")
	time.Sleep(time.Second)
	a.act("server." + server.ID + ".menu.config")
	time.Sleep(2 * time.Second)
	a.shot("11-server-config")

	a.noPageErrors()
}
