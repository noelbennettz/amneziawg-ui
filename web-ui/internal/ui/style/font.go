package style

import (
	"strings"

	"fyne.io/fyne/v2"
)

// The page's text faces.
//
// Fyne's own faces cover Latin, Greek and Cyrillic and stop there - NotoSans,
// Inter, an emoji font and a monospace one - and the wasm build has no system
// fonts behind them to fill a gap. A Chinese string would therefore draw as a
// row of empty boxes, which is what the panel looked like in Chinese before
// Noto Sans SC was shipped with it. One face covers Simplified and Traditional
// alike (30 890 glyphs), so it serves both Chinese translations and any Chinese
// a person types into a server or client name.
//
// The two files weigh about 17 MB together and they used to sit in the bundle,
// so every browser downloaded them whichever language it asked for. They now
// live next to the bundle as font/*.otf and main.go fetches them only for a
// browser whose languages need CJK; see NeedsCJK and SetFaces. A tab that never
// loads them keeps Fyne's stock faces, which is correct for English, French,
// Spanish and Russian and a row of boxes for Chinese.
//
// The files themselves are still in this directory - the build copies them into
// web-ui/wasm/font/ - and the OFL licence sits next to them.
var (
	textFace fyne.Resource
	boldFace fyne.Resource
)

// SetFaces installs the CJK faces once their bytes have arrived. Until it is
// called the theme answers with Fyne's own faces, so a failed download leaves a
// working panel with boxes where Chinese would be rather than no panel at all.
//
// Both files are Noto Sans SC; a nil argument leaves that weight unset.
func SetFaces(regular, bold []byte) {
	// fyne keys its font cache by resource name, so the names stay short and
	// stable: they are what a later SetTheme call looks the parsed face up by.
	if regular != nil {
		textFace = fyne.NewStaticResource("NotoSansSC-Regular.otf", regular)
	}
	if bold != nil {
		boldFace = fyne.NewStaticResource("NotoSansSC-Bold.otf", bold)
	}
}

// NeedsCJK reports whether the CJK face has to be fetched for a browser that
// lists these languages in navigator.languages, most wanted first.
//
// Chinese, Japanese and Korean all need it: the Chinese translations are the
// obvious case, but Fyne also ships base translations for Japanese, and its own
// string there would be boxes without a face that carries kana. Noto Sans SC
// carries kana and Hangul as well as Han, so one face covers all three.
//
// The whole list is consulted rather than its first entry. go-locale hands
// Fyne every language the browser prefers, Fyne matches them and can land on
// Chinese from a list that starts elsewhere - "de-DE, zh-TW" resolves to
// Chinese - and a deduplicated answer here would then leave a Chinese panel
// without a Chinese face. The price is over-fetching for a browser that lists a
// CJK language but gets an English panel, which is rare and costs bandwidth
// once, where the other mistake costs boxes on screen.
func NeedsCJK(languages []string) bool {
	for _, tag := range languages {
		switch primarySubtag(tag) {
		case "zh", "ja", "ko":
			return true
		}
	}
	return false
}

// primarySubtag is the language part of a BCP 47 tag, lowercased: "zh-Hant-TW"
// answers "zh". Anything that is not a language tag - an empty string, "*" -
// answers "".
func primarySubtag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" || tag == "*" {
		return ""
	}
	primary, _, _ := strings.Cut(tag, "-")
	return primary
}
