package manager

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"amneziawg-web-ui/internal/amnezialink"
	"amneziawg-web-ui/internal/awg"
	"amneziawg-web-ui/internal/netutil"
	"amneziawg-web-ui/internal/wgconf"
	"amneziawg-web-ui/web-ui/api"
)

// Clients returns detached copies of every client, optionally narrowed to
// one server.
func (m *Manager) Clients(serverID string) []api.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	servers := m.cfg.Servers
	if serverID != "" {
		srv := m.findServer(serverID)
		if srv == nil {
			return []api.Client{}
		}
		servers = []api.Server{*srv}
	}

	clients := []api.Client{}
	for i := range servers {
		for j := range servers[i].Clients {
			c := cloneClient(&servers[i].Clients[j])
			// Fields a config written before they existed may still be
			// missing; the UI expects both to be set.
			if c.Status == "" {
				c.Status = "active"
			}
			if c.ISettings == nil {
				c.ISettings = map[string]string{}
			}
			clients = append(clients, c)
		}
	}
	return clients
}

// ClientConfig renders the .conf a client imports, with or without the
// header comments.
func (m *Manager) ClientConfig(serverID, clientID string, includeComments bool) (string, error) {
	srv, client, err := m.serverAndClient(serverID, clientID)
	if err != nil {
		return "", err
	}
	return wgconf.ClientConf(&srv, &client, m.endpointFor(&srv), includeComments), nil
}

// ClientLink renders the vpn:// link for a client.
func (m *Manager) ClientLink(serverID, clientID string) (string, error) {
	srv, client, err := m.serverAndClient(serverID, clientID)
	if err != nil {
		return "", err
	}
	return amnezialink.Build(&srv, &client, m.endpointFor(&srv))
}

// serverAndClient snapshots both halves of a client export.
func (m *Manager) serverAndClient(serverID, clientID string) (api.Server, api.Client, error) {
	srv, ok := m.Server(serverID)
	if !ok {
		return api.Server{}, api.Client{}, serverNotFound(serverID)
	}
	client, ok := m.Client(serverID, clientID)
	if !ok {
		return api.Server{}, api.Client{}, clientNotFound(clientID)
	}
	return srv, client, nil
}

// clientConfigOf renders the config for a client the caller already holds a
// copy of - the one an update just produced.
func (m *Manager) clientConfigOf(serverID string, client *api.Client) string {
	srv, ok := m.Server(serverID)
	if !ok {
		return ""
	}
	return wgconf.ClientConf(&srv, client, m.endpointFor(&srv), true)
}

// AddClient adds a WireGuard peer to a server and returns the client with
// its rendered config.
func (m *Manager) AddClient(serverID string, req api.AddClientRequest) (*api.Client, string, error) {
	// The name lands in a .conf comment, in the peer marker and in a
	// Content-Disposition filename, so it is cleaned before anything stores
	// it - not at each of those points.
	name := wgconf.SanitizeName(req.Name, "client")

	// Checked before any key is generated: a malformed signature packet or
	// AllowedIPs list would otherwise reach the client's .conf and only
	// surface there, as a tunnel that will not start.
	if problem := api.ValidateAllowedIPs(req.AllowedIPs); problem != "" {
		return nil, "", fmt.Errorf("%w: %s", ErrInvalid, problem)
	}
	if req.ApplyISettings {
		if err := validateISettings(req.ISettings); err != nil {
			return nil, "", fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}

	// Key generation shells out to awg three times. Doing that under the
	// write lock would stall every other request, including plain reads, for
	// the duration - and the keys do not depend on any config state.
	keys := m.tools.GenerateKeyPair()
	psk := m.tools.GeneratePresharedKey()

	client, ifaceName, err := m.addClientLocked(serverID, name, req, keys, psk)
	if err != nil {
		return nil, "", err
	}

	m.saveOrLog("new client")
	if err := m.syncLiveConfig(ifaceName); err != nil {
		return nil, "", notApplied(err)
	}

	fmt.Printf("Client %s added with AllowedIPs: %s\n", name, client.AllowedIPs)
	return client, m.clientConfigOf(serverID, client), nil
}

// addClientLocked builds the client, appends it to the server's peer config
// file and to the in-memory config, all under a single write lock.
func (m *Manager) addClientLocked(serverID, name string, req api.AddClientRequest, keys awg.KeyPair, psk string) (client *api.Client, ifaceName string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	srv := m.findServer(serverID)
	if srv == nil {
		return nil, "", serverNotFound(serverID)
	}

	clientIP := m.nextClientIP(srv)
	if clientIP == "" {
		return nil, "", fmt.Errorf("subnet is full: %w", ErrConflict)
	}
	clientIPV6 := m.nextClientIPV6(srv)
	if srv.SubnetV6 != "" && clientIPV6 == "" {
		return nil, "", fmt.Errorf("IPv6 subnet %s is full: %w", srv.SubnetV6, ErrConflict)
	}

	iSettings := map[string]string{}
	if req.ApplyISettings {
		iSettings = wgconf.MergeISettings(req.ISettings)
	}

	newClient := api.Client{
		ID:                 m.nextClientID(),
		Name:               name,
		ServerID:           serverID,
		ServerName:         srv.Name,
		Status:             "active",
		CreatedAt:          float64(time.Now().Unix()),
		ClientPrivateKey:   keys.Private,
		ClientPublicKey:    keys.Public,
		PresharedKey:       psk,
		ClientIP:           clientIP,
		ClientIPV6:         clientIPV6,
		ObfuscationEnabled: srv.ObfuscationEnabled,
		ObfuscationParams:  cloneObfuscationParams(srv.ObfuscationParams),
		ApplyISettings:     req.ApplyISettings,
		ISettings:          iSettings,
		AllowedIPs:         wgconf.AllowedIPsOrDefault(req.AllowedIPs),
	}

	// The server side routes only the client's own addresses to it - both
	// stacks - whatever the client itself sends through the tunnel.
	if err := wgconf.AppendPeer(srv.ConfigPath, &newClient, wgconf.PeerAllowedIPs(&newClient)); err != nil {
		return nil, "", fmt.Errorf("failed to write client to server config: %w", err)
	}

	srv.Clients = append(srv.Clients, newClient)

	// newClient is already a detached copy; the caller gets it rather than a
	// pointer into srv.Clients, which the next append could move.
	return &newClient, srv.Interface, nil
}

// nextClientIP hands out an address in the server's subnet, reusing one a
// deleted client gave back before allocating a fresh one. An entry that is no
// longer inside the subnet - a config hand-edited after a subnet change - is
// dropped rather than handed out. Caller holds mu.
func (m *Manager) nextClientIP(srv *api.Server) string {
	for len(srv.UnboundNATIPs) > 0 {
		ip := srv.UnboundNATIPs[0]
		srv.UnboundNATIPs = srv.UnboundNATIPs[1:]
		if netutil.IPInSubnet(ip, srv.Subnet) {
			return ip
		}
	}

	used := map[string]bool{srv.ServerIP: true}
	for _, c := range srv.Clients {
		used[c.ClientIP] = true
	}
	return netutil.NextFreeIP(srv.Subnet, used)
}

// nextClientIPV6 hands out an address in the server's IPv6 subnet, reusing one
// a deleted client gave back before allocating a fresh one. Empty when the
// server has no IPv6 subnet at all or the subnet is full. Caller holds mu.
func (m *Manager) nextClientIPV6(srv *api.Server) string {
	if srv.SubnetV6 == "" {
		return ""
	}
	for len(srv.UnboundNATIPsV6) > 0 {
		ip := srv.UnboundNATIPsV6[0]
		srv.UnboundNATIPsV6 = srv.UnboundNATIPsV6[1:]
		if netutil.IPInSubnet(ip, srv.SubnetV6) {
			return ip
		}
	}

	used := map[string]bool{srv.ServerIPV6: true}
	for _, c := range srv.Clients {
		used[c.ClientIPV6] = true
	}
	return netutil.NextFreeIPv6(srv.SubnetV6, used)
}

// nextClientID draws a client ID no client of any server holds. The six hex
// characters are what the peer marker and the parked file are keyed on, and
// the parked directory is shared by every server, so a collision is not just
// two rows opened by the same ID - it can have one server's parked peer
// deleted with another's client. Caller holds mu.
func (m *Manager) nextClientID() string {
	for {
		id := uuid.New().String()[:6]
		if !m.clientIDTaken(id) {
			return id
		}
	}
}

// clientIDTaken reports whether any server's client list already uses id.
func (m *Manager) clientIDTaken(id string) bool {
	for i := range m.cfg.Servers {
		for j := range m.cfg.Servers[i].Clients {
			if m.cfg.Servers[i].Clients[j].ID == id {
				return true
			}
		}
	}
	return false
}

// DeleteClient removes a peer from a server.
func (m *Manager) DeleteClient(serverID, clientID string) error {
	serverName, clientCopy, ifaceName, err := m.deleteClientLocked(serverID, clientID)
	if err != nil {
		return err
	}

	m.saveOrLog("client removal")
	if err := m.syncLiveConfig(ifaceName); err != nil {
		return notApplied(err)
	}

	fmt.Printf("Client %s:%s removed\n", serverName, clientCopy.Name)
	return nil
}

// deleteClientLocked drops the client from the server's peer list and from
// its .conf under a single write lock, so the file and the config cannot
// disagree about which peers exist. It returns copies only - no pointer into
// the config outlives the lock.
func (m *Manager) deleteClientLocked(serverID, clientID string) (serverName string, clientCopy api.Client, ifaceName string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	srv, target := m.findClient(serverID, clientID)
	if srv == nil {
		return "", api.Client{}, "", serverNotFound(serverID)
	}
	if target == nil {
		return "", api.Client{}, "", clientNotFound(clientID)
	}

	clientCopy = cloneClient(target)
	if _, err := wgconf.RemovePeer(srv.ConfigPath, target); err != nil {
		return "", api.Client{}, "", fmt.Errorf("removing the peer from %s: %w", srv.ConfigPath, err)
	}
	// A suspended client has no block in the live config, only a parked copy;
	// deleting the client has to take that too, or the peer's public key, its
	// preshared key and its addresses stay on the volume for good.
	if err := wgconf.ForgetPeer(srv.ConfigPath, clientID); err != nil {
		// The peer is out of the live config already, so a parked file that
		// will not go away is a line in the log, not a failed deletion.
		fmt.Printf("ForgetPeer %s: %v\n", clientID, err)
	}

	newClients := make([]api.Client, 0, len(srv.Clients)-1)
	for _, c := range srv.Clients {
		if c.ID != clientID {
			newClients = append(newClients, c)
		}
	}
	srv.Clients = newClients
	if ip := clientCopy.ClientIP; ip != "" && !slices.Contains(srv.UnboundNATIPs, ip) {
		srv.UnboundNATIPs = append(srv.UnboundNATIPs, ip)
	}
	if ip := clientCopy.ClientIPV6; ip != "" && !slices.Contains(srv.UnboundNATIPsV6, ip) {
		srv.UnboundNATIPsV6 = append(srv.UnboundNATIPsV6, ip)
	}

	return srv.Name, clientCopy, srv.Interface, nil
}

// UpdateClientAllowedIPs changes the AllowedIPs field for a client.
func (m *Manager) UpdateClientAllowedIPs(serverID, clientID, allowedIPs string) (*api.Client, string, error) {
	if problem := api.ValidateAllowedIPs(allowedIPs); problem != "" {
		return nil, "", fmt.Errorf("%w: %s", ErrInvalid, problem)
	}
	clientCopy, err := m.updateClientLocked(serverID, clientID, func(client *api.Client) {
		client.AllowedIPs = wgconf.AllowedIPsOrDefault(allowedIPs)
	})
	if err != nil {
		return nil, "", err
	}

	m.saveOrLog("client update")
	return clientCopy, m.clientConfigOf(serverID, clientCopy), nil
}

// ParseSuspendAt reads the timestamp the editor and the API send: RFC 3339.
// A value without a zone is read as UTC, which is what the old API accepted.
// The result is the epoch seconds the config stores.
func ParseSuspendAt(value string) (float64, error) {
	stamp, err := time.Parse(time.RFC3339, value)
	if err != nil {
		stamp, err = time.Parse("2006-01-02T15:04:05", value)
		if err != nil {
			return 0, fmt.Errorf("%w: invalid datetime format", ErrInvalid)
		}
	}
	return float64(stamp.Unix()), nil
}

// UpdateClient changes everything about a client the editor can change in one
// write. The three narrower updates still exist, but a dialog that sends them
// one after the other leaves half of an edit applied when the second is
// refused - with this one the request either applies or it does not.
func (m *Manager) UpdateClient(serverID, clientID string, req api.UpdateClientRequest) (*api.Client, string, error) {
	if req.AllowedIPs != nil {
		if problem := api.ValidateAllowedIPs(*req.AllowedIPs); problem != "" {
			return nil, "", fmt.Errorf("%w: %s", ErrInvalid, problem)
		}
	}
	if req.ISettings != nil {
		if err := validateISettings(req.ISettings); err != nil {
			return nil, "", fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}

	suspendSet := req.SuspendAt != nil
	var suspendAt *float64
	if suspendSet && *req.SuspendAt != "" {
		when, err := ParseSuspendAt(*req.SuspendAt)
		if err != nil {
			return nil, "", err
		}
		suspendAt = &when
	}

	clientCopy, err := m.updateClientLocked(serverID, clientID, func(client *api.Client) {
		if req.AllowedIPs != nil {
			client.AllowedIPs = wgconf.AllowedIPsOrDefault(*req.AllowedIPs)
		}
		if req.ApplyISettings != nil {
			client.ApplyISettings = *req.ApplyISettings
		}
		if req.ISettings != nil {
			if client.ApplyISettings {
				client.ISettings = wgconf.MergeISettings(client.ISettings, req.ISettings)
			} else {
				client.ISettings = map[string]string{}
			}
		}
		if suspendSet {
			client.SuspendAt = suspendAt
		}
	})
	if err != nil {
		return nil, "", err
	}

	m.saveOrLog("client update")
	return clientCopy, m.clientConfigOf(serverID, clientCopy), nil
}

// UpdateClientISettings updates the I1-I5 settings for a client.
func (m *Manager) UpdateClientISettings(serverID, clientID string, applyI *bool, iSettings map[string]string) (*api.Client, string, error) {
	if err := validateISettings(iSettings); err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	clientCopy, err := m.updateClientLocked(serverID, clientID, func(client *api.Client) {
		if applyI != nil {
			client.ApplyISettings = *applyI
		}
		if iSettings == nil {
			return
		}
		if client.ApplyISettings {
			client.ISettings = wgconf.MergeISettings(client.ISettings, iSettings)
		} else {
			client.ISettings = map[string]string{}
		}
	})
	if err != nil {
		return nil, "", err
	}

	m.saveOrLog("client update")
	return clientCopy, m.clientConfigOf(serverID, clientCopy), nil
}

// updateClientLocked applies edit to the client under a single write lock
// and returns a copy of the result.
func (m *Manager) updateClientLocked(serverID, clientID string, edit func(*api.Client)) (*api.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	srv, client := m.findClient(serverID, clientID)
	if srv == nil {
		return nil, serverNotFound(serverID)
	}
	if client == nil {
		return nil, clientNotFound(clientID)
	}

	edit(client)

	clientCopy := cloneClient(client)
	return &clientCopy, nil
}
