package style

import (
	"embed"

	"fyne.io/fyne/v2"
)

// The page's text faces.
//
// Fyne's own faces cover Latin, Greek and Cyrillic and stop there - NotoSans,
// Inter, an emoji font and a monospace one - and the wasm build has no system
// fonts behind them to fill a gap. A Chinese string therefore drew as a row of
// empty boxes, which is what the panel looked like in Chinese before these two
// files existed. NotoSansSC is the same family with the CJK repertoire in it
// (30 890 glyphs: Simplified and Traditional alike), so one face serves both
// Chinese translations and any Chinese a person types into a server or client
// name, whatever language their browser asked for.
//
// They are the reason the image grew by about 17 MB; a subset with only the
// 15 615 characters of GB2312 and Big5 would have cost half of that and left
// the rarer characters of a name as boxes, so the whole font ships instead.
// The OFL licence sits next to them.
//
//go:embed font/NotoSansSC-Regular.otf font/NotoSansSC-Bold.otf
var fontFiles embed.FS

var (
	textFace = loadFace("font/NotoSansSC-Regular.otf")
	boldFace = loadFace("font/NotoSansSC-Bold.otf")
)

// loadFace reads one of the embedded faces. A failure here is a broken build,
// not a runtime condition, so it panics the way the login template does.
func loadFace(name string) fyne.Resource {
	data, err := fontFiles.ReadFile(name)
	if err != nil {
		panic("style: " + err.Error())
	}
	// fyne keys its font cache by resource name, so the name stays short.
	return fyne.NewStaticResource(name[len("font/"):], data)
}
