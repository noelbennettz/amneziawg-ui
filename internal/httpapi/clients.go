package httpapi

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"

	"amneziawg-web-ui/internal/manager"
	"amneziawg-web-ui/internal/wgconf"
	"amneziawg-web-ui/web-ui/api"
)

func (h *Handlers) getServerClients(c fiber.Ctx) error {
	return c.JSON(h.mgr.Clients(c.Params("id")))
}

func (h *Handlers) getAllClients(c fiber.Ctx) error {
	return c.JSON(h.mgr.Clients(""))
}

func (h *Handlers) addClient(c fiber.Ctx) error {
	id := c.Params("id")
	var req api.AddClientRequest
	if err := decode(c, &req); err != nil {
		return fail(c, err)
	}
	if req.Name == "" {
		req.Name = "New Client"
	}

	client, configContent, err := h.mgr.AddClient(id, req)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(api.ClientResult{Client: client, Config: configContent})
}

func (h *Handlers) deleteClient(c fiber.Ctx) error {
	id := c.Params("id")
	clientID := c.Params("clientId")
	if err := h.mgr.DeleteClient(id, clientID); err != nil {
		return fail(c, err)
	}
	return c.JSON(api.ActionResult{Status: "deleted", ServerID: id, ClientID: clientID})
}

func (h *Handlers) updateClientAllowedIPs(c fiber.Ctx) error {
	id := c.Params("id")
	clientID := c.Params("clientId")
	var req api.UpdateAllowedIPsRequest
	if err := decode(c, &req); err != nil {
		return fail(c, err)
	}
	client, cfg, err := h.mgr.UpdateClientAllowedIPs(id, clientID, req.AllowedIPs)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(api.ClientResult{Client: client, Config: cfg})
}

func (h *Handlers) updateClientISettings(c fiber.Ctx) error {
	id := c.Params("id")
	clientID := c.Params("clientId")
	var req api.UpdateISettingsRequest
	if err := decode(c, &req); err != nil {
		return fail(c, err)
	}
	client, cfg, err := h.mgr.UpdateClientISettings(id, clientID, req.ApplyISettings, req.ISettings)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(api.ClientResult{Client: client, Config: cfg})
}

func (h *Handlers) downloadClientConfig(c fiber.Ctx) error {
	id := c.Params("id")
	clientID := c.Params("clientId")

	client, ok := h.mgr.Client(id, clientID)
	if !ok {
		return fail(c, fmt.Errorf("client %s: %w", clientID, manager.ErrNotFound))
	}
	configContent, err := h.mgr.ClientConfig(id, clientID, true)
	if err != nil {
		return fail(c, err)
	}

	// Both names are sanitised on the way in, but this one goes into a
	// quoted header value, so it is not left to that alone.
	filename := fmt.Sprintf("%s_%s.conf", wgconf.SanitizeName(client.Name, "client"), wgconf.SanitizeName(client.ServerName, "server"))
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Set("Content-Type", "text/plain; charset=utf-8")
	return c.SendString(configContent)
}

func (h *Handlers) getClientAmneziaLink(c fiber.Ctx) error {
	vpnURL, err := h.mgr.ClientLink(c.Params("id"), c.Params("clientId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(api.AmneziaLink{VPNURL: vpnURL})
}

func (h *Handlers) getClientConfigBoth(c fiber.Ctx) error {
	id := c.Params("id")
	clientID := c.Params("clientId")

	client, ok := h.mgr.Client(id, clientID)
	if !ok {
		return fail(c, fmt.Errorf("client %s: %w", clientID, manager.ErrNotFound))
	}
	clean, err := h.mgr.ClientConfig(id, clientID, false)
	if err != nil {
		return fail(c, err)
	}
	full, err := h.mgr.ClientConfig(id, clientID, true)
	if err != nil {
		return fail(c, err)
	}

	suspendAtReadable := ""
	if client.SuspendAt != nil {
		suspendAtReadable = time.Unix(int64(*client.SuspendAt), 0).UTC().String()
	}

	return c.JSON(api.ClientConfigs{
		ServerID:          id,
		ClientID:          clientID,
		ClientName:        client.Name,
		CleanConfig:       clean,
		FullConfig:        full,
		CleanLength:       len(clean),
		FullLength:        len(full),
		CreatedAt:         client.CreatedAt,
		CreatedAtReadable: time.Unix(int64(client.CreatedAt), 0).UTC().String(),
		SuspendAt:         client.SuspendAt,
		SuspendAtReadable: suspendAtReadable,
	})
}

func (h *Handlers) suspendClient(c fiber.Ctx) error {
	id := c.Params("id")
	clientID := c.Params("clientId")
	msg, err := h.mgr.SuspendClient(id, clientID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(api.ActionResult{Status: "suspended", ServerID: id, ClientID: clientID, Message: msg})
}

func (h *Handlers) activateClient(c fiber.Ctx) error {
	id := c.Params("id")
	clientID := c.Params("clientId")
	msg, err := h.mgr.ActivateClient(id, clientID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(api.ActionResult{Status: "activated", ServerID: id, ClientID: clientID, Message: msg})
}

func (h *Handlers) updateClientSuspendTime(c fiber.Ctx) error {
	id := c.Params("id")
	clientID := c.Params("clientId")

	var req api.UpdateSuspendTimeRequest
	if err := decode(c, &req); err != nil {
		return fail(c, err)
	}

	var ts *float64
	if req.SuspendAt != nil && *req.SuspendAt != "" {
		t, err := time.Parse(time.RFC3339, *req.SuspendAt)
		if err != nil {
			// Try without timezone
			t, err = time.Parse("2006-01-02T15:04:05", *req.SuspendAt)
			if err != nil {
				return respond(c, fiber.StatusBadRequest,
					api.ErrorResponse{Error: "invalid datetime format"})
			}
		}
		v := float64(t.Unix())
		ts = &v
	}

	client, msg, err := h.mgr.UpdateClientSuspendTime(id, clientID, ts)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(api.ClientResult{Client: client, Message: msg})
}
