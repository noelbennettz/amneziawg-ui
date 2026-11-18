package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// The page in front of the panel speaks the browser's language, the same six
// the Fyne bundle is translated into. The header is what a browser sends: a
// list of tags, most wanted first, optionally with q-values.
func TestLoginTextFor(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", "Sign in"},
		{"en-US,en;q=0.9", "Sign in"},
		{"ru-RU,ru;q=0.9,en-US;q=0.8", "Войти"},
		{"fr-CA,fr;q=0.9", "Se connecter"},
		{"es-MX,es;q=0.9", "Iniciar sesión"},
		{"zh-CN,zh;q=0.9", "登录"},
		{"zh-TW,zh;q=0.9", "登入"},
		{"zh-HK", "登入"},
		{"zh-Hant-TW", "登入"},
		{"zh", "登录"},
		// The preferred language is the one with the highest q, whatever
		// order the list is in, and a language the panel lacks is skipped
		// rather than swallowing the whole header.
		{"de-DE,es;q=0.8", "Iniciar sesión"},
		{"ru;q=0.4,fr;q=0.9", "Se connecter"},
		{"de-DE", "Sign in"},
		{"*", "Sign in"},
		// q=0 means "not acceptable", and a broken q is not a preference.
		{"ru;q=0,es;q=0.5", "Iniciar sesión"},
		{"ru;q=not-a-number,es;q=0.5", "Iniciar sesión"},
	}

	for _, c := range cases {
		if got := loginTextFor(c.header).SignIn; got != c.want {
			t.Errorf("Accept-Language %q: Sign in = %q, want %q", c.header, got, c.want)
		}
	}
}

// The four strings and the lang attribute reach the page. A Chinese browser
// gets Chinese labels, and the error after a wrong pair is in the same
// language as the form around it.
func TestLoginPageSpeaksTheBrowsersLanguage(t *testing.T) {
	app := newTestApp(t)

	cases := []struct {
		header string
		lang   string
		user   string
		signIn string
	}{
		{"zh-TW,zh;q=0.9", `lang="zh-TW"`, "使用者名稱", "登入"},
		{"zh-CN", `lang="zh-Hans"`, "用户名", "登录"},
		{"fr-FR", `lang="fr"`, "Nom d&#39;utilisateur", "Se connecter"},
	}
	for _, c := range cases {
		page := get(t, app, LoginPath, nil, map[string]string{"Accept-Language": c.header})
		html := body(t, page)
		for _, want := range []string{c.lang, c.user, c.signIn} {
			if !strings.Contains(html, want) {
				t.Errorf("Accept-Language %q: the page does not contain %q", c.header, want)
			}
		}
	}

	// A wrong pair re-renders the form: the error has to be translated too,
	// which is the whole reason the wording is picked per request.
	token, cookie := csrfToken(t, app)
	req := httptest.NewRequest(http.MethodPost, LoginPath, strings.NewReader(url.Values{
		"username": {"admin"},
		"password": {"wrong"},
		"csrf":     {token},
	}.Encode()))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	req.Header.Set("Accept-Language", "zh-TW")
	req.AddCookie(cookie)
	html := body(t, do(t, app, req))
	if !strings.Contains(html, "使用者名稱或密碼不正確。") {
		t.Error("the 401 does not carry the translated error")
	}
}
