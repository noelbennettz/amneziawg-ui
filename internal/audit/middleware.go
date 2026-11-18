package audit

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

// UserLocalKey is where the guard puts the name it authenticated, so a change
// recorded here can say who made it. IPLocalKey is the address it resolved for
// the same request: behind a proxy that is the forwarded one, which only the
// guard knows how to read.
const (
	UserLocalKey = "audit.user"
	IPLocalKey   = "audit.ip"
)

// Middleware records every change that reached the API: the method, the route,
// where it came from, who was logged in and how it ended. Reads are not
// recorded - a panel polls its status every 30 seconds, and a log of that is
// noise, not evidence.
//
// It goes after the guard, which is what fills the user in, and after CSRF, so
// a request that was refused before it got that far is not a change.
func Middleware(log *Log) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()
		if log == nil {
			return err
		}

		switch c.Method() {
		case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		default:
			return err
		}
		// Only the API. The login, the logout and a rejected write are
		// recorded where they are decided, with the reason attached.
		if !strings.HasPrefix(c.Path(), "/api/") {
			return err
		}

		status := c.Response().StatusCode()
		if err != nil {
			status = fiber.StatusInternalServerError
		}

		log.Record(Event{
			Event:  EventChange,
			User:   UserLocal(c),
			IP:     IPLocal(c),
			Method: c.Method(),
			Path:   c.Path(),
			Status: status,
		})
		return err
	}
}

// UserLocal is the authenticated name the guard left on the request.
func UserLocal(c fiber.Ctx) string {
	user, _ := c.Locals(UserLocalKey).(string)
	return user
}

// IPLocal is the address the guard resolved for the request - the forwarded
// one when WEB_UI_TRUST_PROXY is on - falling back to the connection's own.
func IPLocal(c fiber.Ctx) string {
	if ip, ok := c.Locals(IPLocalKey).(string); ok && ip != "" {
		return ip
	}
	return c.IP()
}
