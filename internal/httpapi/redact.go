// Redaction for the JSON surface.
//
// The private keys live in web_config.json and in the .conf files, and the
// config endpoints hand them out because that is what they are for. A list,
// a status answer or the result of an edit is not: the panel never reads
// those fields, so they have no business crossing the wire on every poll -
// and a comment claiming they never leave the backend has to be true.

package httpapi

import "amneziawg-web-ui/web-ui/api"

// redactServer blanks the server private key, and the key inside every client
// carried along with it.
func redactServer(s api.Server) api.Server {
	s.ServerPrivateKey = ""
	s.Clients = redactClients(s.Clients)
	return s
}

// redactClient blanks one client's private key.
func redactClient(c api.Client) api.Client {
	c.ClientPrivateKey = ""
	return c
}

// redactClientPtr is redactClient for the pointer the manager hands back.
func redactClientPtr(c *api.Client) *api.Client {
	if c == nil {
		return nil
	}
	redacted := redactClient(*c)
	return &redacted
}

// redactClients applies redactClient to a list, keeping it non-nil so the
// response still renders [] rather than null.
func redactClients(clients []api.Client) []api.Client {
	out := make([]api.Client, len(clients))
	for i := range clients {
		out[i] = redactClient(clients[i])
	}
	return out
}
