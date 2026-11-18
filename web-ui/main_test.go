package main

import (
	"encoding/json"
	"path"
	"testing"

	"fyne.io/fyne/v2/lang"
)

// loadTranslations loads the bundle the way main does, for a browser that
// reports the given locale. go-locale reads the environment on Linux (LANGUAGE
// first, then LC_ALL/LANG), and AddTranslationsFS re-picks the locale after
// loading, so the variables have to be in place before the call.
func loadTranslations(t *testing.T, locale string) {
	t.Helper()

	t.Setenv("LC_ALL", locale)
	t.Setenv("LANGUAGE", locale)

	if err := lang.AddTranslationsFS(translations, "translation"); err != nil {
		t.Fatalf("loading translations: %v", err)
	}
}

// The browser's locale has to land on the file that speaks it: a French or
// Spanish browser used to fall back to English, and Chinese needs both of its
// scripts.
//
// The Traditional file is named zh-TW.json rather than zh-Hant.json on
// purpose. fyne picks the language in lang.closestSupportedLocale and passes
// the result through lang.localeFromTag, which only ever returns
// base[-region[-script]] and drops the script when there is no region - so
// "zh-Hant" comes back as plain "zh" and go-i18n then resolves that to
// Simplified. A region survives the trip, so the file carries TW and a zh-TW,
// zh-HK, zh-Hant or zh-Hant-TW browser all reach it.
func TestTranslationsPickTheMatchingLanguage(t *testing.T) {
	cases := []struct {
		locale string
		want   string
	}{
		{"en_GB.UTF-8", "Start"},
		{"ru_RU.UTF-8", "Запустить"},
		{"fr_CA.UTF-8", "Démarrer"},
		{"es_MX.UTF-8", "Iniciar"},
		// Simplified Chinese, then Traditional: the script subtag is what
		// tells them apart, and the region is what a browser actually sends.
		{"zh_CN.UTF-8", "启动"},
		{"zh_SG.UTF-8", "启动"},
		{"zh_TW.UTF-8", "啟動"},
		{"zh_HK.UTF-8", "啟動"},
		{"zh_Hant.UTF-8", "啟動"},
		// A language the panel does not ship falls back to English rather
		// than to whichever file happens to share a script with it.
		{"de_DE.UTF-8", "Start"},
	}

	for _, c := range cases {
		loadTranslations(t, c.locale)
		if got := lang.L("Start"); got != c.want {
			t.Errorf("%s: Start = %q, want %q", c.locale, got, c.want)
		}
	}
}

// A substitution, and the plural forms: Russian has three of them, Chinese a
// single one, which is what the "other"-only entries in its files say.
func TestTranslationsApplyPluralsAndTemplates(t *testing.T) {
	loadTranslations(t, "ru_RU.UTF-8")
	cases := []struct{ got, want string }{
		{lang.L("Server \"{{.Name}}\" started", map[string]any{"Name": "vpn"}), "Сервер «vpn» запущен"},
		{lang.N("{{.Count}} clients", 1, map[string]any{"Count": 1}), "1 клиент"},
		{lang.N("{{.Count}} clients", 3, map[string]any{"Count": 3}), "3 клиента"},
		{lang.N("{{.Count}} clients", 5, map[string]any{"Count": 5}), "5 клиентов"},
		{lang.N("{{.Count}} clients", 21, map[string]any{"Count": 21}), "21 клиент"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}

	loadTranslations(t, "zh_TW.UTF-8")
	if got, want := lang.N("{{.Count}} clients", 3, map[string]any{"Count": 3}), "3 個用戶端"; got != want {
		t.Errorf("traditional Chinese plural: got %q, want %q", got, want)
	}
}

// Every key the English file has, every other language has too, and no
// language invents one - otherwise a string silently falls back to English for
// one language only, or a dead key stays behind in the files.
func TestTranslationsHaveTheSameKeys(t *testing.T) {
	keys := func(name string) map[string]bool {
		data, err := translations.ReadFile("translation/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var file map[string]json.RawMessage
		if err := json.Unmarshal(data, &file); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		set := make(map[string]bool, len(file))
		for k := range file {
			set[k] = true
		}
		return set
	}

	entries, err := translations.ReadDir("translation")
	if err != nil {
		t.Fatal(err)
	}

	en := keys("en.json")
	if len(en) == 0 {
		t.Fatal("en.json is empty")
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == "en.json" || path.Ext(name) != ".json" {
			continue
		}
		other := keys(name)
		for k := range en {
			if !other[k] {
				t.Errorf("%s lacks %q", name, k)
			}
		}
		for k := range other {
			if !en[k] {
				t.Errorf("en.json lacks %q, which %s translates", k, name)
			}
		}
	}
}
