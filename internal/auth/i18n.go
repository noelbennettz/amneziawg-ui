package auth

import (
	"strconv"
	"strings"
)

// loginText is the wording of the one page this package renders. It is not the
// Fyne bundle the panel uses: that one lives in the wasm binary, which a
// browser asking for /login has not downloaded yet (and cannot, before it has
// a session), so the four strings are carried here as well.
type loginText struct {
	// Lang is what the page declares in <html lang="...">.
	Lang     string
	Username string
	Password string
	SignIn   string
	Error    string
	// TooMany is what a blocked address is shown instead: the limit is not a
	// wrong password, and telling somebody their password is wrong when it is
	// not sends them looking for a problem that does not exist.
	TooMany string
}

// loginTexts is every language the panel is translated into, keyed by the tag
// picked below. The keys match the file names in web-ui/translation, except
// that Traditional Chinese is "zh-TW" for the same reason the file is - see
// the comment on TestTranslationsPickTheMatchingLanguage in web-ui.
var loginTexts = map[string]loginText{
	"en": {
		Lang: "en", Username: "Username", Password: "Password", SignIn: "Sign in",
		Error:   "Incorrect username or password.",
		TooMany: "Too many failed logins. Try again later.",
	},
	"ru": {
		Lang: "ru", Username: "Имя пользователя", Password: "Пароль", SignIn: "Войти",
		Error:   "Неверное имя пользователя или пароль.",
		TooMany: "Слишком много неудачных попыток входа. Попробуйте позже.",
	},
	"fr": {
		Lang: "fr", Username: "Nom d'utilisateur", Password: "Mot de passe", SignIn: "Se connecter",
		Error:   "Nom d'utilisateur ou mot de passe incorrect.",
		TooMany: "Trop de tentatives de connexion échouées. Réessayez plus tard.",
	},
	"es": {
		Lang: "es", Username: "Usuario", Password: "Contraseña", SignIn: "Iniciar sesión",
		Error:   "Usuario o contraseña incorrectos.",
		TooMany: "Demasiados intentos de inicio de sesión fallidos. Inténtalo más tarde.",
	},
	"zh-Hans": {
		Lang: "zh-Hans", Username: "用户名", Password: "密码", SignIn: "登录",
		Error:   "用户名或密码不正确。",
		TooMany: "登录失败次数过多，请稍后再试。",
	},
	"zh-TW": {
		Lang: "zh-TW", Username: "使用者名稱", Password: "密碼", SignIn: "登入",
		Error:   "使用者名稱或密碼不正確。",
		TooMany: "登入失敗次數過多，請稍後再試。",
	},
}

// defaultLoginText is what a browser that asks for nothing gets.
var defaultLoginText = loginTexts["en"]

// loginTextFor picks the wording for a request's Accept-Language header. The
// header is a list of tags with optional q-values, most wanted first; the
// highest q that the panel speaks wins, and English is the fallback - the same
// rule the Fyne side follows, so the page and the panel agree.
func loginTextFor(header string) loginText {
	best, bestQ := defaultLoginText, 0.0

	for _, entry := range strings.Split(header, ",") {
		tag, params, _ := strings.Cut(entry, ";")
		tag, params = strings.TrimSpace(tag), strings.TrimSpace(params)

		q := 1.0
		if value, ok := strings.CutPrefix(params, "q="); ok {
			// A malformed q is treated as "wanted least", which keeps it
			// from outranking an entry the browser actually asked for.
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				continue
			}
			q = parsed
		}
		if q <= 0 || q <= bestQ {
			continue
		}

		if text, ok := loginTexts[languageTag(tag)]; ok {
			best, bestQ = text, q
		}
	}

	return best
}

// languageTag turns a header tag into one of the keys above: the language
// alone for the languages that have one written form, and zh-Hans or zh-TW for
// Chinese, whose script a browser states either outright or as a region.
func languageTag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" || tag == "*" {
		return ""
	}

	parts := strings.FieldsFunc(tag, func(r rune) bool { return r == '-' || r == '_' })

	switch {
	case parts[0] == "zh":
		for _, part := range parts[1:] {
			switch part {
			case "hant", "tw", "hk", "mo":
				return "zh-TW"
			case "hans", "cn", "sg", "my":
				return "zh-Hans"
			}
		}
		// A bare "zh" is Simplified the way the CLDR defaults it.
		return "zh-Hans"
	case parts[0] == "en", parts[0] == "ru", parts[0] == "fr", parts[0] == "es":
		return parts[0]
	}

	return ""
}
