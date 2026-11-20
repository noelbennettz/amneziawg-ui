package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"amneziawg-web-ui/internal/awg"
	"amneziawg-web-ui/internal/awg/awgtest"
	"amneziawg-web-ui/internal/config"
	"amneziawg-web-ui/internal/manager"
	"amneziawg-web-ui/internal/store"
	"amneziawg-web-ui/internal/wgconf"
	"amneziawg-web-ui/web-ui/api"
)

// newTestAPI wires the real handlers onto a Fiber app backed by a manager
// loaded from a temp dir holding one server, "srv", whose interface is down
// and whose host commands all fail until the test stubs them.
func newTestAPI(t *testing.T) (*fiber.App, *manager.Manager, *awgtest.Runner) {
	t.Helper()
	dir := t.TempDir()
	confPath := filepath.Join(dir, "wg-s1.conf")
	if err := os.WriteFile(confPath, []byte("[Interface]\nPrivateKey = PRIV\nS1 = 50\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	st := store.New(filepath.Join(dir, "web_config.json"))
	err := st.Save(&store.AppConfig{SchemaVersion: store.SchemaVersion, Servers: []api.Server{{
		ID: "s1", Name: "srv", Interface: "wg-test-absent", ConfigPath: confPath,
		ServerPrivateKey: "SERVERPRIV",
		Subnet:           "10.0.1.0/24", ServerIP: "10.0.1.1", MTU: 1280, Port: 54844,
		Status: "stopped", ObfuscationEnabled: true,
		ObfuscationParams: &api.ObfuscationParams{S1: 50, S2: 60, S3: 20, S4: 16, MTU: 1280},
		Clients:           []api.Client{},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	run := awgtest.New()
	tools := awg.New(run)
	tools.ScriptsDir = filepath.Join(dir, "no-scripts")
	settings := config.Settings{
		WebUIPort: 54845, DefaultMTU: 1280, DefaultSubnet: "10.0.0.0/24", DefaultPort: 54844,
		DNSServers: []string{"8.8.8.8"}, EnableObfuscation: true,
		ConfigFile: st.Path(), WireguardConfigDir: dir,
	}
	m, err := manager.New(settings, st, tools)
	if err != nil {
		t.Fatalf("manager.New: %v", err)
	}

	app := fiber.New(FiberConfig())
	New(m).RegisterRoutes(app)
	return app, m, run
}

func call(t *testing.T, app *fiber.App, method, path, body string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}

func addClient(t *testing.T, m *manager.Manager, name string, applyI bool) *api.Client {
	t.Helper()
	client, _, err := m.AddClient("s1", api.AddClientRequest{Name: name, ApplyISettings: applyI})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// "No such server" and "the interface refused to come up" used to be the same
// answer: 404 with "server not found or failed to start".
func TestMissingServerIs404AndAFailedStartIs500(t *testing.T) {
	app, _, _ := newTestAPI(t)

	status, body := call(t, app, http.MethodPost, "/api/servers/nope/start", "")
	if status != http.StatusNotFound {
		t.Errorf("unknown server: status = %d, want 404 (%s)", status, body)
	}

	// s1 exists; awg-quick is not installed here, so bringing it up fails.
	status, body = call(t, app, http.MethodPost, "/api/servers/s1/start", "")
	if status != http.StatusInternalServerError {
		t.Errorf("failed start: status = %d, want 500 (%s)", status, body)
	}
	var errResp api.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil || errResp.Error == "" {
		t.Errorf("error payload = %s (%v)", body, err)
	}
}

// Creating a server used to answer 400 whatever went wrong. A port another
// server already listens on is a clash with the current state, not a
// malformed request, and an invalid MTU is still the request's own fault.
func TestCreateServerSeparatesAPortClashFromABadValue(t *testing.T) {
	app, _, _ := newTestAPI(t) // "srv" is already on 54844

	status, body := call(t, app, http.MethodPost, "/api/servers",
		`{"name":"second","port":54844}`)
	if status != http.StatusConflict {
		t.Errorf("duplicate port: status = %d, want 409 (%s)", status, body)
	}
	if !strings.Contains(string(body), "54844") {
		t.Errorf("error payload should name the port: %s", body)
	}

	status, body = call(t, app, http.MethodPost, "/api/servers",
		`{"name":"second","port":54846,"mtu":9000}`)
	if status != http.StatusBadRequest {
		t.Errorf("bad MTU: status = %d, want 400 (%s)", status, body)
	}
}

// A list, a status answer or the result of an edit carries no private key.
// They live in web_config.json and in the .conf the config endpoints hand out,
// and the panel never reads them from these payloads - sending them on every
// dashboard poll put every key on a wire it had no business on.
func TestResponsesDoNotCarryPrivateKeys(t *testing.T) {
	app, m, _ := newTestAPI(t)
	client := addClient(t, m, "alice", false)
	if client.ClientPrivateKey == "" {
		t.Fatal("the fixture produced a client with no private key")
	}

	for _, path := range []string{"/api/servers", "/api/clients", "/api/servers/s1/clients", "/api/servers/s1/info"} {
		status, body := call(t, app, http.MethodGet, path, "")
		if status != http.StatusOK {
			t.Fatalf("GET %s: status = %d (%s)", path, status, body)
		}
		for _, secret := range []string{"SERVERPRIV", client.ClientPrivateKey} {
			if strings.Contains(string(body), secret) {
				t.Errorf("GET %s carries a private key: %s", path, body)
			}
		}
	}

	status, body := call(t, app, http.MethodPost, "/api/servers/s1/clients", `{"name":"bob"}`)
	if status != http.StatusOK {
		t.Fatalf("create client: status = %d (%s)", status, body)
	}
	var created api.ClientResult
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("create client payload = %s (%v)", body, err)
	}
	if created.Client == nil || created.Client.ClientPrivateKey != "" {
		t.Errorf("the create result carries the client's private key: %s", body)
	}
	if created.Config == "" {
		t.Error("the create result lost the config the dialog shows")
	}
}

// An unknown server used to answer the client list with 200 and [], which
// reads as "that server has no clients" for a stale ID.
func TestClientsOfAnUnknownServerAre404(t *testing.T) {
	app, _, _ := newTestAPI(t)

	status, body := call(t, app, http.MethodGet, "/api/servers/nope/clients", "")
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (%s)", status, body)
	}
}

func TestMalformedBodyIsRejected(t *testing.T) {
	app, m, _ := newTestAPI(t)
	client := addClient(t, m, "alice", false)

	for _, path := range []string{
		"/api/servers/s1/clients",
		"/api/servers/s1/clients/" + client.ID + "/allowed-ips",
	} {
		method := http.MethodPost
		if strings.HasSuffix(path, "allowed-ips") {
			method = http.MethodPut
		}
		status, body := call(t, app, method, path, "{not json")
		if status != http.StatusBadRequest {
			t.Errorf("%s %s: status = %d, want 400 (%s)", method, path, status, body)
		}
	}

	// A body that parses but leaves everything at its default is still fine.
	status, _ := call(t, app, http.MethodPut,
		"/api/servers/s1/clients/"+client.ID+"/allowed-ips", `{}`)
	if status != http.StatusOK {
		t.Errorf("empty object: status = %d, want 200", status)
	}
}

func TestActivatingAnActiveClientIsAConflict(t *testing.T) {
	app, m, _ := newTestAPI(t)
	client := addClient(t, m, "alice", false)

	status, body := call(t, app, http.MethodPost,
		"/api/servers/s1/clients/"+client.ID+"/activate", "")
	if status != http.StatusConflict {
		t.Errorf("status = %d, want 409 (%s)", status, body)
	}
}

// The frontend parses these payloads into the types in web-ui/api; the
// backend must produce exactly those shapes.
func TestResponsesMatchTheDeclaredPayloads(t *testing.T) {
	app, m, _ := newTestAPI(t)
	client := addClient(t, m, "alice", true)

	status, body := call(t, app, http.MethodGet, "/api/servers/s1/info", "")
	if status != http.StatusOK {
		t.Fatalf("info: status = %d (%s)", status, body)
	}
	var info api.ServerInfo
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatal(err)
	}
	if info.ID != "s1" || info.ClientsCount != 1 || len(info.Clients) != 1 {
		t.Errorf("ServerInfo = %+v", info)
	}
	if info.DefaultISettings["i1"] != wgconf.DefaultI1 {
		t.Errorf("default I-settings missing from ServerInfo")
	}
	if info.Status != "stopped" {
		t.Errorf("status = %q, want the observed stopped", info.Status)
	}

	status, body = call(t, app, http.MethodGet,
		"/api/servers/s1/clients/"+client.ID+"/config-both", "")
	if status != http.StatusOK {
		t.Fatalf("config-both: status = %d (%s)", status, body)
	}
	var configs api.ClientConfigs
	if err := json.Unmarshal(body, &configs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(configs.CleanConfig, "[Interface]") || configs.FullLength == 0 {
		t.Errorf("ClientConfigs = %+v", configs)
	}

	status, body = call(t, app, http.MethodGet, "/api/servers/s1/clients/"+client.ID+"/link", "")
	if status != http.StatusOK {
		t.Fatalf("link: status = %d (%s)", status, body)
	}
	var link api.AmneziaLink
	if err := json.Unmarshal(body, &link); err != nil || !strings.HasPrefix(link.VPNURL, "vpn://") {
		t.Errorf("AmneziaLink = %s (%v)", body, err)
	}

	status, body = call(t, app, http.MethodGet, "/api/servers/s1/config", "")
	if status != http.StatusOK {
		t.Fatalf("server config: status = %d (%s)", status, body)
	}
	var serverConfig api.ServerConfig
	if err := json.Unmarshal(body, &serverConfig); err != nil || !strings.Contains(serverConfig.ConfigContent, "S1 = 50") {
		t.Errorf("ServerConfig = %s (%v)", body, err)
	}

	status, body = call(t, app, http.MethodDelete, "/api/servers/s1/clients/"+client.ID, "")
	if status != http.StatusOK {
		t.Fatalf("delete: status = %d (%s)", status, body)
	}
	var action api.ActionResult
	if err := json.Unmarshal(body, &action); err != nil {
		t.Fatal(err)
	}
	if action.Status != "deleted" || action.ClientID != client.ID {
		t.Errorf("ActionResult = %+v", action)
	}

	status, body = call(t, app, http.MethodGet, "/api/system/status", "")
	if status != http.StatusOK {
		t.Fatalf("system status: status = %d (%s)", status, body)
	}
	var sys api.SystemStatus
	if err := json.Unmarshal(body, &sys); err != nil {
		t.Fatal(err)
	}
	if sys.TotalServers != 1 || sys.TotalClients != 0 || sys.Environment.WebUIPort != "54845" {
		t.Errorf("SystemStatus = %+v", sys)
	}
}

// Starting a server whose interface is already up is a conflict, not a
// failure: awg-quick would just error out with "File exists".
func TestStartingARunningServerIsAConflict(t *testing.T) {
	app, _, run := newTestAPI(t)
	run.Stub("ip link show wg-test-absent", "wg-test-absent: state UNKNOWN")

	status, body := call(t, app, http.MethodPost, "/api/servers/s1/start", "")
	if status != http.StatusConflict {
		t.Errorf("start: status = %d, want 409 (%s)", status, body)
	}

	run.Unstub("ip link show wg-test-absent")
	// The observation is cached briefly; a fresh manager sees the interface gone.
	app2, _, _ := newTestAPI(t)
	status, body = call(t, app2, http.MethodPost, "/api/servers/s1/stop", "")
	if status != http.StatusConflict {
		t.Errorf("stop: status = %d, want 409 (%s)", status, body)
	}
}

// A client's ServerID is taken straight off the route and kept in the config
// for as long as the client exists, which is exactly the shape that broke
// once: fiber hands route parameters back as strings pointing into the
// request buffer it recycles after the handler returns, so without
// FiberConfig's Immutable the stored id came back as a slice of an unrelated
// later request ("i-sett", off some .../i-settings path) while the file on
// disk, written before the recycling, still looked right.
//
// app.Test does not recycle contexts the way a real listener does, so the
// traffic below does not reproduce that corruption - only a running server
// did. This guards the invariant the fix restored, not the mechanism.
func TestStoredClientServerIDSurvivesLaterRequests(t *testing.T) {
	app, m, _ := newTestAPI(t)

	status, body := call(t, app, http.MethodPost, "/api/servers/s1/clients", `{"name":"laptop"}`)
	if status != http.StatusOK {
		t.Fatalf("add client: %d %s", status, body)
	}

	var added api.ClientResult
	if err := json.Unmarshal(body, &added); err != nil {
		t.Fatal(err)
	}

	for range 20 {
		call(t, app, http.MethodPut, "/api/servers/s1/clients/"+added.Client.ID+"/i-settings",
			`{"apply_i_settings":false}`)
		call(t, app, http.MethodGet, "/api/servers/s1/info", "")
	}

	stored, ok := m.Client("s1", added.Client.ID)
	if !ok {
		t.Fatal("client vanished from its server")
	}
	if stored.ServerID != "s1" {
		t.Errorf("stored ServerID = %q, want \"s1\"", stored.ServerID)
	}
}

// The iptables check and the health check must not depend on the host: one
// is answered from the runner, the other from /proc.
func TestSystemEndpointsAnswerWithoutTheHost(t *testing.T) {
	app, _, run := newTestAPI(t)
	run.Stub("iptables -S INPUT", "-A INPUT -i wg-test-absent -j ACCEPT")

	status, body := call(t, app, http.MethodGet, "/api/system/iptables-test?server_id=s1", "")
	if status != http.StatusOK {
		t.Fatalf("iptables-test: status = %d (%s)", status, body)
	}
	var check api.IptablesTest
	if err := json.Unmarshal(body, &check); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, v := range check.IptablesCheck {
		if v == "Found" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("IptablesCheck = %v, want exactly one rule found", check.IptablesCheck)
	}
	// No script is deployed in the test tree, so the per-family verdict is
	// unknown rather than a guess.
	if check.NAT4.State != "unknown" || check.NAT6.State != "unknown" {
		t.Errorf("NAT states = %q/%q, want unknown/unknown", check.NAT4.State, check.NAT6.State)
	}

	status, body = call(t, app, http.MethodGet, "/api/system/iptables-test", "")
	if status != http.StatusBadRequest {
		t.Errorf("iptables-test without server_id: status = %d (%s)", status, body)
	}

	status, body = call(t, app, http.MethodGet, "/status", "")
	if status != http.StatusOK || !strings.HasPrefix(string(body), "Container Uptime:") {
		t.Errorf("/status = %d %s", status, body)
	}
}

// /version answers the build's identity, and it answers with real fields even
// in a test binary nobody passed ldflags to: a bug report needs an answer, not
// an empty object.
func TestVersionEndpoint(t *testing.T) {
	app, _, _ := newTestAPI(t)

	status, body := call(t, app, http.MethodGet, "/api/version", "")
	if status != http.StatusOK {
		t.Fatalf("/api/version = %d %s", status, body)
	}

	var info struct {
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		BuildTime string `json:"build_time"`
		Go        string `json:"go"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("/api/version payload = %s (%v)", body, err)
	}
	for name, value := range map[string]string{
		"version": info.Version,
		"commit":  info.Commit,
		"built":   info.BuildTime,
		"go":      info.Go,
	} {
		if value == "" {
			t.Errorf("%s is empty in %s", name, body)
		}
	}
	if !strings.HasPrefix(info.Go, "go") {
		t.Errorf("go = %q, want the toolchain version", info.Go)
	}
}

// The endpoint of a server that already exists can be changed now: a VPS whose
// address moved, or a name the operator only pointed here after the fact.
func TestUpdateServerEndpoint(t *testing.T) {
	app, _, _ := newTestAPI(t)

	status, body := call(t, app, http.MethodPut, "/api/servers/s1", `{"endpoint":"vpn.example.com"}`)
	if status != http.StatusOK {
		t.Fatalf("PUT server: status = %d (%s)", status, body)
	}
	_, servers := call(t, app, http.MethodGet, "/api/servers", "")
	if !strings.Contains(string(servers), `"endpoint":"vpn.example.com"`) {
		t.Errorf("the endpoint did not stick: %s", servers)
	}

	status, _ = call(t, app, http.MethodPut, "/api/servers/s1", `{"endpoint":"vpn.example.com:54844"}`)
	if status != http.StatusBadRequest {
		t.Errorf("an endpoint carrying a port: status = %d, want 400", status)
	}
	status, _ = call(t, app, http.MethodPut, "/api/servers/nope", `{"endpoint":"vpn.example.com"}`)
	if status != http.StatusNotFound {
		t.Errorf("unknown server: status = %d, want 404", status)
	}
}

// The client editor sends one request; a refused value has to leave every
// field as it was, not apply the rest of the edit.
func TestUpdateClientInOneRequest(t *testing.T) {
	app, m, _ := newTestAPI(t)
	client := addClient(t, m, "alice", false)

	status, body := call(t, app, http.MethodPut, "/api/servers/s1/clients/"+client.ID,
		`{"allowed_ips":"10.0.0.0/8","apply_i_settings":false,"suspend_at":""}`)
	if status != http.StatusOK {
		t.Fatalf("PUT client: status = %d (%s)", status, body)
	}
	if !strings.Contains(string(body), `"allowed_ips":"10.0.0.0/8"`) {
		t.Errorf("the update result does not carry the new routes: %s", body)
	}

	status, _ = call(t, app, http.MethodPut, "/api/servers/s1/clients/"+client.ID,
		`{"allowed_ips":"10.1.0.0/16","suspend_at":"not-a-time"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("bad timestamp: status = %d, want 400", status)
	}
	stored, ok := m.Client("s1", client.ID)
	if !ok || stored.AllowedIPs != "10.0.0.0/8" {
		t.Errorf("a refused update changed AllowedIPs to %q", stored.AllowedIPs)
	}

	status, _ = call(t, app, http.MethodPut, "/api/servers/s1/clients/"+client.ID, `{"allowed_ips":"nope"}`)
	if status != http.StatusBadRequest {
		t.Errorf("garbage AllowedIPs: status = %d, want 400", status)
	}
}

// Detection goes out to the network and can rewrite the config, so it is not
// something a link prefetch or a scanner can set off with a GET.
func TestRefreshIPIsAPost(t *testing.T) {
	app, _, _ := newTestAPI(t)

	status, _ := call(t, app, http.MethodGet, "/api/system/refresh-ip", "")
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/system/refresh-ip = %d, want 405", status)
	}
}
