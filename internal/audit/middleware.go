package audit

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

// UserLocalKey is where the guard puts the name it authenticated, so a change
// recorded here can say who made it.
const UserLocalKey = "audit.user"

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
			IP:     c.IP(),
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
