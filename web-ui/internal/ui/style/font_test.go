package style

import (
	"bytes"
	"embed"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/go-text/typesetting/font"
)

// The production build no longer embeds the faces - main.go fetches them for a
// browser that needs them - but the files still ship with the source, and these
// tests load them from there. Embedding them here rather than reading the
// directory keeps the tests running under the js/wasm interpreter the frontend
// suite uses, where there is no filesystem to read.
//
//go:embed font/NotoSansSC-Regular.otf font/NotoSansSC-Bold.otf
var testFaces embed.FS

// The two files are the only reason a Chinese string is not a row of empty
// boxes: fyne's own faces stop at Latin, Greek and Cyrillic, and the wasm build
// has no system fonts to borrow a glyph from. This pins the repertoire the
// translations rely on, so that a font swapped for a cheaper one - or a file
// dropped from the build - fails here instead of on screen.
func TestTextFacesCoverTheTranslatedScripts(t *testing.T) {
	for _, name := range []string{"font/NotoSansSC-Regular.otf", "font/NotoSansSC-Bold.otf"} {
		data, err := testFaces.ReadFile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		parsed, err := font.ParseTTF(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		// One string per script the panel is translated into, plus the Chinese
		// characters a user is most likely to type into a name.
		const sample = "VPN 客户端服务器" + // Chinese, Simplified
			"用戶端伺服器" + // Chinese, Traditional
			"Запустить" + // Russian
			"Créer" + "Añadir" + // French, Spanish
			"Activar"

		for _, r := range sample {
			if _, ok := parsed.NominalGlyph(r); !ok {
				t.Errorf("%s has no glyph for %q", name, r)
			}
		}
	}
}

// Until the faces have been fetched the theme has to answer with Fyne's own,
// so a browser that needs no CJK - or one whose download failed - still gets a
// working panel. Once they are installed every style that carries text gets
// one of them.
func TestThemeUsesTheCJKFacesOnlyOnceLoaded(t *testing.T) {
	textFace, boldFace = nil, nil
	t.Cleanup(func() { textFace, boldFace = nil, nil })

	th := NewTheme()
	stock := theme.DefaultTheme().Font(fyne.TextStyle{})
	if got := th.Font(fyne.TextStyle{}); got != stock {
		t.Errorf("before loading, Font() = %v, want the stock face %v", got, stock)
	}
	regular, err := testFaces.ReadFile("font/NotoSansSC-Regular.otf")
	if err != nil {
		t.Fatal(err)
	}
	bold, err := testFaces.ReadFile("font/NotoSansSC-Bold.otf")
	if err != nil {
		t.Fatal(err)
	}
	SetFaces(regular, bold)
	for _, style := range []fyne.TextStyle{{}, {Bold: true}, {Italic: true}, {Bold: true, Italic: true}} {
		if got := th.Font(style); got != textFace && got != boldFace {
			t.Errorf("Font(%+v) = %v, want one of the CJK faces", style, got)
		}
	}
	if got := th.Font(fyne.TextStyle{Bold: true}); got != boldFace {
		t.Error("bold text does not get the bold face")
	}
	// The config view is an ASCII .conf and its columns are the monospace
	// advance; the symbols are the icons. Neither may be replaced by Noto.
	for _, style := range []fyne.TextStyle{{Monospace: true}, {Symbol: true}} {
		if got := th.Font(style); got == textFace || got == boldFace {
			t.Errorf("Font(%+v) answered with the CJK face", style)
		}
	}
}

// Which browsers pay for the 17 MB of CJK glyphs. The list is every language
// the browser prefers, not just the first: Fyne matches the whole list and can
// land on Chinese from a list that starts with German.
func TestNeedsCJK(t *testing.T) {
	cases := []struct {
		languages []string
		want      bool
	}{
		{[]string{"en-US", "en"}, false},
		{[]string{"fr-CA"}, false},
		{[]string{"es-ES", "en"}, false},
		{[]string{"ru-RU", "en"}, false},
		{[]string{"de-DE"}, false},
		{[]string{"zh-TW"}, true},
		{[]string{"zh-Hant-HK"}, true},
		{[]string{"zh-CN", "en"}, true},
		{[]string{"zh"}, true},
		{[]string{"de-DE", "zh-TW"}, true},
		{[]string{"ja-JP"}, true},
		{[]string{"ko-KR"}, true},
		{[]string{"ZH-hans"}, true},
		{[]string{}, false},
		{[]string{"*"}, false},
		{[]string{"", "en"}, false},
	}

	for _, c := range cases {
		if got := NeedsCJK(c.languages); got != c.want {
			t.Errorf("NeedsCJK(%q) = %v, want %v", c.languages, got, c.want)
		}
	}
}
