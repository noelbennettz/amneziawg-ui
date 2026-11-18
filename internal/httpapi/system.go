package httpapi

import (
	"fmt"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"

	"amneziawg-web-ui/internal/wgconf"
	"amneziawg-web-ui/web-ui/api"
)

// containerUptime is the open health check. PID 1's /proc entry was created
// when the container started, so its modification time is the boot time.
func (h *Handlers) containerUptime(c fiber.Ctx) error {
	info, err := os.Stat("/proc/1/cmdline")
	if err != nil {
		return c.SendString("Container Uptime: unknown")
	}
	uptime := int64(time.Since(info.ModTime()).Seconds())
	d := uptime / 86400
	h2 := (uptime % 86400) / 3600
	m2 := (uptime % 3600) / 60
	s2 := uptime % 60
	return c.SendString(fmt.Sprintf("Container Uptime: %dd %dh %dm %ds", d, h2, m2, s2))
}

func (h *Handlers) getDefaultISettings(c fiber.Ctx) error {
	return c.JSON(wgconf.DefaultISettings())
}

func (h *Handlers) systemStatus(c fiber.Ctx) error {
	return c.JSON(h.mgr.SystemStatus())
}

func (h *Handlers) refreshIP(c fiber.Ctx) error {
	return c.JSON(api.PublicIP{Address: h.mgr.RefreshPublicIP()})
}

func (h *Handlers) iptablesTest(c fiber.Ctx) error {
	serverID := c.Query("server_id")
	if serverID == "" {
		return respond(c, fiber.StatusBadRequest,
			api.ErrorResponse{Error: "server_id parameter required"})
	}
	result, err := h.mgr.IPTablesCheck(serverID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(result)
}
