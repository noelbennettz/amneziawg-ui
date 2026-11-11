package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/pprof"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"amneziawg-web-ui/internal/audit"
	"amneziawg-web-ui/internal/auth"
	"amneziawg-web-ui/internal/awg"
	"amneziawg-web-ui/internal/config"
	"amneziawg-web-ui/internal/frontend"
	"amneziawg-web-ui/internal/httpapi"
	"amneziawg-web-ui/internal/manager"
	"amneziawg-web-ui/internal/security"
	"amneziawg-web-ui/internal/store"
)

// staticDir is where "make web-ui" leaves the packaged frontend, relative to
// the working directory the server is started from: the repository root in
// development and /app in the container (see the WORKDIR in the Dockerfile).
const staticDir = "./web-ui/wasm"

func main() {
	fmt.Println("AmneziaWG Web UI (Go/Fiber) starting...")

	settings := config.FromEnv()
	settings.EnsureDirectories()

	mgr := manager.New(settings, store.New(settings.ConfigFile), awg.New(awg.Shell{}))
	mgr.Start()

	// The frontend is opened before the app so the nonce its loader page is
	// rewritten around can go into the policy the app answers with.
	nonce := security.NewNonce()
	assets := frontend.Open(os.DirFS(staticDir), nonce)

	app := fiber.New(httpapi.FiberConfig())

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(security.Headers(assets.Nonce()))
	app.Use(assets.Compression())

	// Off unless WEB_UI_AUDIT_LOG is set: the log names the addresses that
	// reached the panel, and keeping that record is the operator's call.
	auditLog := audit.New(settings.AuditLog, config.AuditFile, settings.AuditRetention)

	// Everything except the health check and the login page is behind the
	// login: the browser is sent to /login, which hands out a session cookie
	// once the form is posted with the credentials the container was started
	// with. An Authorization: Basic header is still accepted, so scripts keep
	// working with "curl -u".
	guard := auth.New(settings.User, settings.PasswordHash,
		auth.WithSecureCookies(settings.SecureCookie),
		auth.WithAudit(auditLog),
		auth.WithLoginRateLimit(settings.LoginRateLimit),
		auth.WithTrustProxy(settings.TrustProxy),
	)
	app.Use(guard.Middleware())
	// A write has to prove it came from this page, not from one that made
	// the browser send the cookie. Registered after the guard, so a request
	// without credentials is still answered 401 rather than 403.
	app.Use(guard.CSRF())
	app.Use(audit.Middleware(auditLog))
	guard.Register(app)

	// Profiling is opt-in: collecting a CPU profile costs the running server
	// real time, and the endpoints hand out stack traces and command line of
	// the process. Registered after the guard on purpose, so /debug/pprof
	// requires the same credentials as everything else.
	if settings.Pprof {
		app.Use(pprof.New())
		fmt.Println("pprof enabled at /debug/pprof/")
	}

	// REST routes first, so the asset catch-all cannot shadow them.
	httpapi.New(mgr).RegisterRoutes(app)
	assets.Mount(app)

	fmt.Printf("Serving the frontend from %s\n", staticDir)
	addr := ":" + strconv.Itoa(settings.WebUIPort)
	if settings.WebUIBind != "" {
		addr = net.JoinHostPort(settings.WebUIBind, strconv.Itoa(settings.WebUIPort))
	}
	fmt.Printf("Listening on %s\n", addr)
	if settings.AuditLog {
		fmt.Printf("Audit log: %s\n", config.AuditFile)
	}
	if settings.TrustProxy {
		fmt.Println("Client addresses come from X-Forwarded-For (WEB_UI_TRUST_PROXY)")
	}
	if settings.SecureCookie == config.SecureOn {
		fmt.Println("Cookies will be marked Secure: the panel must be reached over HTTPS")
	}
	log.Fatal(app.Listen(addr, httpapi.ListenConfig()))
}
