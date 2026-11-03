package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/basicauth"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/pprof"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"amneziawg-web-ui/internal/awg"
	"amneziawg-web-ui/internal/config"
	"amneziawg-web-ui/internal/frontend"
	"amneziawg-web-ui/internal/httpapi"
	"amneziawg-web-ui/internal/manager"
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

	app := fiber.New(httpapi.FiberConfig())

	app.Use(recover.New())
	app.Use(logger.New())

	assets := frontend.Open(os.DirFS(staticDir))
	app.Use(assets.Compression())

	// Basic auth protects everything except the health check: every asset of
	// the UI is behind the same credentials as the API.
	app.Use(basicauth.New(basicauth.Config{
		Next: func(c fiber.Ctx) bool {
			return c.Path() == "/status"
		},
		Users: map[string]string{
			settings.User: settings.PasswordHash,
		},
		Realm: "Restricted Content",
	}))

	// Profiling is opt-in: collecting a CPU profile costs the running server
	// real time, and the endpoints hand out stack traces and command line of
	// the process. Registered after basicauth on purpose, so /debug/pprof
	// requires the same credentials as everything else.
	if settings.Pprof {
		app.Use(pprof.New())
		fmt.Println("pprof enabled at /debug/pprof/")
	}

	// REST routes first, so the asset catch-all cannot shadow them.
	httpapi.New(mgr).RegisterRoutes(app)
	assets.Mount(app)

	fmt.Printf("Serving the frontend from %s\n", staticDir)
	fmt.Printf("Listening on :%d\n", settings.WebUIPort)
	log.Fatal(app.Listen(":" + strconv.Itoa(settings.WebUIPort)))
}
