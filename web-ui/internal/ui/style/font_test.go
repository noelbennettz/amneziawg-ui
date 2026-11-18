package style

import (
	"bytes"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/go-text/typesetting/font"
)

// The two files font.go embeds are the only reason a Chinese string is not a
// row of empty boxes: fyne's own faces stop at Latin, Greek and Cyrillic, and
// the wasm build has no system fonts to borrow a glyph from. This pins the
// repertoire the translations rely on, so that a font swapped for a cheaper
// one - or a file dropped on a rebuild - fails here instead of on screen.
func TestTextFacesCoverTheTranslatedScripts(t *testing.T) {
	faces := []struct {
		name string
		res  fyne.Resource
	}{
		{"regular", textFace},
		{"bold", boldFace},
	}

	// One string per script the panel is translated into, plus the Chinese
	// characters a user is most likely to type into a name.
	const sample = "VPN 客户端服务器" + // Chinese, Simplified
		"用戶端伺服器" + // Chinese, Traditional
		"Запустить" + // Russian
		"Créer" + "Añadir" + // French, Spanish
		"Activar"

	for _, face := range faces {
		parsed, err := font.ParseTTF(bytes.NewReader(face.res.Content()))
		if err != nil {
			t.Fatalf("%s face: %v", face.name, err)
		}
		for _, r := range sample {
			if _, ok := parsed.NominalGlyph(r); !ok {
				t.Errorf("%s face has no glyph for %q", face.name, r)
			}
		}
	}
}

// The theme is what the rest of the UI asks for a face, so the wiring is worth
// one assertion: every style that carries text gets a face with CJK in it.
func TestThemeFontIsTheSharedFace(t *testing.T) {
	th := NewTheme()

	for _, style := range []fyne.TextStyle{{}, {Bold: true}, {Italic: true}, {Bold: true, Italic: true}} {
		if got := th.Font(style); got != textFace && got != boldFace {
			t.Errorf("Font(%+v) = %v, want one of the embedded faces", style, got)
		}
	}
	if got := th.Font(fyne.TextStyle{Bold: true}); got != boldFace {
		t.Errorf("bold text does not get the bold face")
	}
}
