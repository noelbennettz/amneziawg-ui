// Command web-ui is the AmneziaWG Web UI frontend: a Fyne application
// compiled to WebAssembly and served, together with its loader page, by the
// Go/Fiber backend it talks to.
package main

import (
	"embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/lang"

	"amneziawg-web-ui/web-ui/internal/fixes"
	"amneziawg-web-ui/web-ui/internal/ui"
)

// The UI strings, one JSON file per language (en.json is the fallback). The
// language is picked from the browser's navigator.languages, so the page
// comes up in the user's language with nothing to configure.
//
//go:embed translation
var translations embed.FS

func main() {
	application := app.NewWithID("io.amnezia.webui")

	if err := lang.AddTranslationsFS(translations, "translation"); err != nil {
		fyne.LogError("loading translations", err)
	}

	// Upstream workarounds next: they have to be in place before the window
	// takes any input, and one of them replaces the app's clipboard.
	fixes.Install(application)

	// One theme, always dark - the page is served with a matching dark
	// loader, so the app never flashes a light background.
	application.Settings().SetTheme(ui.NewDarkTheme())

	window := application.NewWindow("AmneziaWG Web UI")
	frontend := ui.New(application, window)
	window.SetContent(frontend.Build())
	window.Resize(fyne.NewSize(1280, 900))

	// Loading starts once the toolkit is running, so the first fyne.Do calls
	// always find a live event loop.
	application.Lifecycle().SetOnStarted(frontend.Start)

	window.ShowAndRun()
}
