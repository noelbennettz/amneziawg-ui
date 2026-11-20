package manager

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"amneziawg-web-ui/web-ui/api"
)

func TestAddClientDetachesObfuscationParamsFromTheServer(t *testing.T) {
	m, _ := newTestManager(t)

	client, _, err := m.AddClient("s1", api.AddClientRequest{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if client.ObfuscationParams == nil {
		t.Fatal("client got no obfuscation params")
	}

	// Retuning the server must not rewrite what an already-issued client was
	// handed: both ends have to keep agreeing on the values in its config.
	m.cfg.Servers[0].ObfuscationParams.S1 = 999

	stored, ok := m.Client("s1", client.ID)
	if !ok {
		t.Fatal("client vanished from its server")
	}
	if stored.ObfuscationParams.S1 != 50 {
		t.Errorf("server edit leaked into the stored client: S1 = %d, want 50", stored.ObfuscationParams.S1)
	}
	if client.ObfuscationParams.S1 != 50 {
		t.Errorf("server edit leaked into the returned client: S1 = %d, want 50", client.ObfuscationParams.S1)
	}
}

func TestClientLivesOnlyInItsServer(t *testing.T) {
	m, _ := newTestManager(t)

	const signature = "<b 0xcafe><r 20>"

	client, _, err := m.AddClient("s1", api.AddClientRequest{Name: "alice", ApplyISettings: true, ISettings: map[string]string{"i2": signature}})
	if err != nil {
		t.Fatal(err)
	}

	if got := m.ClientCount(); got != 1 {
		t.Errorf("clientCount = %d, want 1", got)
	}
	all := m.Clients("")
	perServer := m.Clients("s1")
	if len(all) != 1 || len(perServer) != 1 {
		t.Fatalf("client listed %d time(s) globally and %d time(s) per server, want 1 and 1", len(all), len(perServer))
	}
	if all[0].ID != client.ID || perServer[0].ID != client.ID {
		t.Errorf("listings disagree on the client: %q / %q, want %q", all[0].ID, perServer[0].ID, client.ID)
	}

	// A returned client is a copy: writing to it must not reach the config.
	all[0].Name = "mutated"
	all[0].ISettings["i2"] = "mutated"
	stored, _ := m.Client("s1", client.ID)
	if stored.Name != "alice" || stored.ISettings["i2"] != signature {
		t.Errorf("caller's edits leaked into the config: %+v", stored)
	}

	if err := m.DeleteClient("s1", client.ID); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}
	if got := m.ClientCount(); got != 0 {
		t.Errorf("clientCount after delete = %d, want 0", got)
	}
	if got := len(m.Clients("")); got != 0 {
		t.Errorf("deleted client still listed %d time(s)", got)
	}
}

func TestSuspendAndActivateKeepASingleClientRecord(t *testing.T) {
	m, _ := newTestManager(t)

	client, _, err := m.AddClient("s1", api.AddClientRequest{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.SuspendClient("s1", client.ID); err != nil {
		t.Fatalf("SuspendClient: %v", err)
	}
	listed := m.Clients("")
	if len(listed) != 1 || listed[0].Status != "suspended" {
		t.Fatalf("after suspend: %+v", listed)
	}

	conf, _ := os.ReadFile(m.cfg.Servers[0].ConfigPath)
	if strings.Contains(string(conf), client.ClientPublicKey) {
		t.Errorf("suspended peer still in the server conf:\n%s", conf)
	}

	if _, err := m.ActivateClient("s1", client.ID); err != nil {
		t.Fatalf("ActivateClient: %v", err)
	}
	listed = m.Clients("")
	if len(listed) != 1 || listed[0].Status != "active" {
		t.Fatalf("after activate: %+v", listed)
	}
}

// Before peer blocks carried the client ID, deleting one of two clients that
// share a name took both peers out of the server .conf.
func TestDeletingOneOfTwoNamesakesKeepsTheOther(t *testing.T) {
	m, _ := newTestManager(t)

	first, _, err := m.AddClient("s1", api.AddClientRequest{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := m.AddClient("s1", api.AddClientRequest{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	if err := m.DeleteClient("s1", first.ID); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}

	conf, err := os.ReadFile(m.cfg.Servers[0].ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(conf), first.ClientPublicKey) {
		t.Errorf("deleted peer still in the conf:\n%s", conf)
	}
	if !strings.Contains(string(conf), second.ClientPublicKey) {
		t.Errorf("the namesake's peer was removed too:\n%s", conf)
	}
	if !strings.Contains(string(conf), "S1 = 50") {
		t.Errorf("[Interface] section damaged:\n%s", conf)
	}
	if got := m.ClientCount(); got != 1 {
		t.Errorf("clientCount = %d, want 1", got)
	}
}

// A name is free text; it must not be able to inject config directives into
// the peer comment it lands in.
func TestClientNameCannotInjectConfigLines(t *testing.T) {
	m, _ := newTestManager(t)

	client, _, err := m.AddClient("s1", api.AddClientRequest{Name: "evil\n[Peer]\nPublicKey = INJECTED"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(client.Name, "\n\r") {
		t.Errorf("stored name still has line breaks: %q", client.Name)
	}

	conf, _ := os.ReadFile(m.cfg.Servers[0].ConfigPath)
	if strings.Contains(string(conf), "INJECTED\n") && !strings.Contains(string(conf), "# Client:") {
		t.Errorf("name broke out of its comment:\n%s", conf)
	}
	if got := strings.Count(string(conf), "[Peer]"); got != 1 {
		t.Errorf("[Peer] blocks = %d, want 1:\n%s", got, conf)
	}
}

// Concurrent reads and writes must not race, and every one of them must be
// accounted for. Meaningful under -race.
func TestConcurrentClientChurnIsRaceFree(t *testing.T) {
	m, _ := newTestManager(t)

	const n = 12
	added := make(chan string, n)
	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client, _, err := m.AddClient("s1", api.AddClientRequest{Name: fmt.Sprintf("client-%d", i)})
			if err != nil {
				t.Errorf("AddClient: %v", err)
				return
			}
			added <- client.ID
		}(i)
	}
	for range n {
		wg.Go(func() {
			m.Clients("")
			m.Clients("s1")
			m.Servers()
			m.ClientCount()
		})
	}
	wg.Wait()
	close(added)

	if got := m.ClientCount(); got != n {
		t.Fatalf("clientCount = %d, want %d", got, n)
	}

	// Every client must have its own address and its own peer block.
	conf, _ := os.ReadFile(m.cfg.Servers[0].ConfigPath)
	if got := strings.Count(string(conf), "[Peer]"); got != n {
		t.Errorf("[Peer] blocks = %d, want %d", got, n)
	}
	ips := map[string]bool{}
	for _, c := range m.Clients("s1") {
		if ips[c.ClientIP] {
			t.Errorf("duplicate client IP %s", c.ClientIP)
		}
		ips[c.ClientIP] = true
	}

	for id := range added {
		if _, ok := m.Client("s1", id); !ok {
			t.Errorf("client %s was lost", id)
		}
	}
}

// A client whose AllowedIPs the engine cannot parse is one whose config will
// not start; the panel must not store it and call the add a success.
func TestAddClientRejectsAnAllowedIPsListTheEngineCannotParse(t *testing.T) {
	m, _ := newTestManager(t)
	before := m.ClientCount()

	if _, _, err := m.AddClient("s1", api.AddClientRequest{Name: "bad", AllowedIPs: "not-a-cidr"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("AddClient error = %v, want ErrInvalid", err)
	}
	if got := m.ClientCount(); got != before {
		t.Errorf("a rejected client was stored: count = %d", got)
	}
}

// The editor sends the whole client in one request now: a value it refuses
// must leave every field as it was, never half of the edit.
func TestUpdateClientAppliesEverythingOrNothing(t *testing.T) {
	m, _ := newTestManager(t)
	client, _, err := m.AddClient("s1", api.AddClientRequest{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	routes := "10.0.0.0/8"
	apply := true
	settings := api.ISettings{"i2": "<b 0xcafe><r 20>"}
	when := "2030-01-02T03:04:05Z"

	updated, cfg, err := m.UpdateClient("s1", client.ID, api.UpdateClientRequest{
		AllowedIPs: &routes, ApplyISettings: &apply, ISettings: settings, SuspendAt: &when,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.AllowedIPs != routes || !updated.ApplyISettings || updated.ISettings["i2"] == "" || updated.SuspendAt == nil {
		t.Errorf("updated client = %+v", updated)
	}
	if !strings.Contains(cfg, "AllowedIPs = 10.0.0.0/8") {
		t.Errorf("the regenerated config does not carry the new routes:\n%s", cfg)
	}

	// A bad timestamp next to a new route: nothing is applied.
	bad := "not-a-time"
	other := "10.1.0.0/16"
	if _, _, err := m.UpdateClient("s1", client.ID, api.UpdateClientRequest{AllowedIPs: &other, SuspendAt: &bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("UpdateClient error = %v, want ErrInvalid", err)
	}
	stored, _ := m.Client("s1", client.ID)
	if stored.AllowedIPs != routes {
		t.Errorf("a refused update changed AllowedIPs to %q", stored.AllowedIPs)
	}

	// An empty string clears the schedule.
	empty := ""
	updated, _, err = m.UpdateClient("s1", client.ID, api.UpdateClientRequest{SuspendAt: &empty})
	if err != nil || updated.SuspendAt != nil {
		t.Fatalf("clearing the schedule = %+v, %v", updated.SuspendAt, err)
	}

	// Garbage routes are refused by the same rule creation uses.
	garbage := "10.0.0.0/8,"
	if _, _, err := m.UpdateClient("s1", client.ID, api.UpdateClientRequest{AllowedIPs: &garbage}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("garbage AllowedIPs error = %v, want ErrInvalid", err)
	}
}

// A change that cannot be pushed onto the running interface is not a success:
// the config is saved, but the tunnel still has the old peer list.
func TestAddClientReportsASyncFailure(t *testing.T) {
	m, run := newTestManager(t)
	run.Stub("ip link show wg-test-absent", "wg-test-absent: state UNKNOWN")

	_, _, err := m.AddClient("s1", api.AddClientRequest{Name: "alice"})
	if err == nil || !strings.Contains(err.Error(), "saved") {
		t.Fatalf("AddClient error = %v, want the stored-but-not-applied failure", err)
	}
	if got := m.ClientCount(); got != 1 {
		t.Errorf("the client was not stored: count = %d", got)
	}
}
