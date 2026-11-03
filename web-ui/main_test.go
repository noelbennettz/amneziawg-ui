package main

import (
	"encoding/json"
	"testing"

	"fyne.io/fyne/v2/lang"
)

// The translation files are data the compiler never looks at: a stray comma,
// a key go-i18n reserves ("description", "other", ...) or a key present in
// one language and missing from the other only shows up on screen. This
// loads the bundle the way main does, as a Russian browser would, and checks
// the translations actually come back.
func TestTranslationsLoadAndApply(t *testing.T) {
	// go-locale reads the language from the environment on Linux; LANGUAGE
	// is what it consults first. AddTranslationsFS re-picks the locale after
	// loading, so the variable has to be in place before the call.
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	t.Setenv("LANGUAGE", "ru")

	if err := lang.AddTranslationsFS(translations, "translation"); err != nil {
		t.Fatalf("loading translations: %v", err)
	}

	// A plain string, one with a substitution, and the plural forms Russian
	// has and English does not.
	cases := []struct{ got, want string }{
		{lang.L("Start"), "Запустить"},
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
}

// Every key the English file has, the Russian one has too, and vice versa -
// otherwise a string silently falls back to English for one language only.
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

	en, ru := keys("en.json"), keys("ru.json")
	for k := range en {
		if !ru[k] {
			t.Errorf("ru.json lacks %q", k)
		}
	}
	for k := range ru {
		if !en[k] {
			t.Errorf("en.json lacks %q", k)
		}
	}
}
