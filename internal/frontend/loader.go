package frontend

import (
	"html"
	"strings"

	"amneziawg-web-ui/internal/version"
)

// The page a browser sees while the wasm bundle is being downloaded.
//
// It is written here rather than taken from the "fyne package" output, which
// this package used to repaint. Two reasons. The packaged page draws its
// progress with an animated GIF - dark and light variants, both machine-drawn -
// and says "Downloading web assembly file" in English; the panel is Material 3
// in dark neutral with a blue accent and is translated into six languages, so
// neither belonged. And the packaged page is a template someone else owns: the
// old code rewrote it with string surgery that had to fall back to
// 'unsafe-inline' whenever fyne changed a marker, which meant a security
// downgrade every time fyne made a release.
//
// Owning the page makes the policy simpler - one inline script, ours, carrying
// the nonce - and leaves one contract with fyne to keep: wasm_exec.js defines
// Go, bundle.wasm is the module, the canvas appears on document.body once the
// app runs, and an input named dummyEntry is what the driver focuses for a soft
// keyboard. AGENTS.md records that contract; a fyne upgrade that changes it
// shows up as a loader that never goes away rather than as a silently broken
// policy.
const loaderHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="color-scheme" content="dark">
<meta name="theme-color" content="#131314">
{{csp}}<title>AmneziaWG Web UI</title>
<link rel="icon" type="image/png" href="icon.png">
<style>
* { box-sizing: border-box; }
html, body { margin: 0; padding: 0; height: 100%; background: #131314; -webkit-tap-highlight-color: transparent; }
body {
	color: #e3e3e3; text-align: center;
	font-family: system-ui, -apple-system, "Segoe UI", Roboto, "Noto Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif;
	-webkit-font-smoothing: antialiased;
}
/* A fixed overlay rather than a flex body: fyne appends its canvas to <body>
   when the app starts, and a flex parent would lay that canvas out as an item
   of its own instead of letting it fill the viewport. */
#loader {
	position: fixed; inset: 0; z-index: 1;
	display: flex; flex-direction: column; align-items: center; justify-content: center;
	padding: 24px;
	transition: opacity .18s ease, visibility 0s linear 0s;
}
#loader.hidden { opacity: 0; visibility: hidden; transition: opacity .18s ease, visibility 0s linear .18s; }
.icon { display: block; width: 96px; height: 96px; border-radius: 28px; margin-bottom: 24px; }
.name { font-size: 22px; line-height: 28px; font-weight: 500; }
.version { margin-top: 4px; font-size: 12px; line-height: 16px; letter-spacing: .5px; color: #c4c7c5; }
.progress { width: 48px; height: 48px; margin: 32px 0 16px; animation: spin 1.4s linear infinite; }
.progress circle { fill: none; stroke-width: 4; }
.track { stroke: #333537; }
.arc {
	stroke: #a8c7fa; stroke-linecap: round;
	stroke-dasharray: 1 200; stroke-dashoffset: 0;
	animation: dash 1.4s ease-in-out infinite;
}
.status { max-width: 40ch; font-size: 14px; line-height: 20px; letter-spacing: .25px; color: #c4c7c5; }
.status.error { color: #ffb4ab; }
#loader.error .progress { display: none; }
@keyframes spin { to { transform: rotate(360deg); } }
@keyframes dash {
	0% { stroke-dasharray: 1 200; stroke-dashoffset: 0; }
	50% { stroke-dasharray: 90 200; stroke-dashoffset: -35px; }
	100% { stroke-dasharray: 90 200; stroke-dashoffset: -124px; }
}
@media (prefers-reduced-motion: reduce) {
	.progress { animation-duration: 4.5s; }
	.arc { animation: none; stroke-dasharray: 120 200; }
}
</style>
</head>
<body>
<div id="loader" aria-busy="true">
<img class="icon" src="icon.png" alt="">
<div class="name">AmneziaWG Web UI</div>
{{version}}<svg class="progress" viewBox="0 0 48 48" aria-hidden="true" focusable="false">
<circle class="track" cx="24" cy="24" r="20"></circle>
<circle class="arc" cx="24" cy="24" r="20"></circle>
</svg>
<div class="status" id="status" role="status">Starting…</div>
<noscript><div class="status error">JavaScript needs to be enabled for this panel.</div></noscript>
</div>
<input id="dummyEntry" style="position: absolute; left: 0; top: -100pt; z-index: -10; font-size: 32pt;">
<script{{nonce}}>
(function () {
	"use strict";

	// The same six languages the panel is translated into, and the same rule
	// the Go side uses to pick one: the browser's list, most wanted first, and
	// the first entry the panel speaks. A browser that speaks none gets English.
	var strings = {
		en: {
			starting: "Starting the panel…",
			loading: "Loading the interface…",
			webgl: "WebGL is disabled in this browser, and the panel draws with it.",
			wasm: "This browser cannot run WebAssembly, which the panel is built with.",
			failed: "The interface could not be loaded. Reload the page to try again."
		},
		ru: {
			starting: "Запуск панели…",
			loading: "Загрузка интерфейса…",
			webgl: "В этом браузере отключён WebGL, которым панель рисует интерфейс.",
			wasm: "Этот браузер не может запускать WebAssembly, на котором построена панель.",
			failed: "Не удалось загрузить интерфейс. Обновите страницу и попробуйте снова."
		},
		fr: {
			starting: "Démarrage du panneau…",
			loading: "Chargement de l'interface…",
			webgl: "WebGL est désactivé dans ce navigateur, et le panneau s'en sert pour dessiner.",
			wasm: "Ce navigateur ne peut pas exécuter WebAssembly, sur lequel repose le panneau.",
			failed: "L'interface n'a pas pu être chargée. Rechargez la page pour réessayer."
		},
		es: {
			starting: "Iniciando el panel…",
			loading: "Cargando la interfaz…",
			webgl: "WebGL está desactivado en este navegador y el panel lo necesita para dibujar.",
			wasm: "Este navegador no puede ejecutar WebAssembly, sobre el que está construido el panel.",
			failed: "No se pudo cargar la interfaz. Recarga la página para volver a intentarlo."
		},
		"zh-Hans": {
			starting: "正在启动面板…",
			loading: "正在加载界面…",
			webgl: "此浏览器已禁用 WebGL，面板需要它来绘制界面。",
			wasm: "此浏览器无法运行 WebAssembly，面板是用它构建的。",
			failed: "界面加载失败，请刷新页面重试。"
		},
		"zh-TW": {
			starting: "正在啟動面板…",
			loading: "正在載入介面…",
			webgl: "此瀏覽器已停用 WebGL，面板需要它來繪製介面。",
			wasm: "此瀏覽器無法執行 WebAssembly，面板是用它建構的。",
			failed: "介面載入失敗，請重新整理頁面再試。"
		}
	};

	function language() {
		var list = navigator.languages && navigator.languages.length
			? navigator.languages
			: [navigator.language || "en"];
		for (var i = 0; i < list.length; i++) {
			var parts = String(list[i]).toLowerCase().replace(/_/g, "-").split("-");
			if (parts[0] === "zh") {
				var traditional = parts.indexOf("hant") >= 0 || parts.indexOf("tw") >= 0 ||
					parts.indexOf("hk") >= 0 || parts.indexOf("mo") >= 0;
				return traditional ? "zh-TW" : "zh-Hans";
			}
			if (parts[0] === "en" || parts[0] === "ru" || parts[0] === "fr" || parts[0] === "es") {
				return parts[0];
			}
		}
		return "en";
	}

	var lang = language();
	var text = strings[lang];
	document.documentElement.setAttribute("lang", lang);

	var loader = document.getElementById("loader");
	var status = document.getElementById("status");
	status.textContent = text.starting;

	function fail(message) {
		loader.className = "error";
		loader.setAttribute("aria-busy", "false");
		status.className = "status error";
		status.textContent = message;
	}

	function webgl() {
		try {
			var canvas = document.createElement("canvas");
			return !!(window.WebGLRenderingContext &&
				(canvas.getContext("webgl") || canvas.getContext("experimental-webgl")));
		} catch (e) {
			return false;
		}
	}

	// The loading screen goes when the canvas it covers arrives. Watching for
	// it is what Fyne's own loader page does, and it is the only signal the
	// page gets: the wasm module never tells the document that it has started.
	function watch() {
		var observer = new MutationObserver(function () {
			if (document.querySelector("canvas")) {
				observer.disconnect();
				loader.className = "hidden";
				loader.setAttribute("aria-busy", "false");
			}
		});
		observer.observe(document.body, { childList: true, subtree: true });
	}

	function run() {
		var go = new Go();
		var instantiate = WebAssembly.instantiateStreaming
			? WebAssembly.instantiateStreaming
			: function (response, object) {
				return response.arrayBuffer().then(function (buffer) {
					return WebAssembly.instantiate(buffer, object);
				});
			};

		status.textContent = text.loading;
		instantiate(fetch("bundle.wasm"), go.importObject).then(function (result) {
			watch();
			go.run(result.instance);
		}).catch(function () {
			fail(text.failed);
		});
	}

	function start() {
		if (!webgl()) {
			fail(text.webgl);
			return;
		}
		if (!window.WebAssembly) {
			fail(text.wasm);
			return;
		}
		var script = document.createElement("script");
		script.src = "wasm_exec.js";
		script.addEventListener("load", run);
		script.addEventListener("error", function () { fail(text.failed); });
		document.head.appendChild(script);
	}

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", start);
	} else {
		start();
	}
})();
</script>
</body>
</html>
`

// loaderPage is the page served for "/" and "/index.html", with the process's
// script nonce in place and the build's release under the app's name.
//
// An empty nonce - a development build, or a server started without one -
// leaves the attribute off; the policy then allows inline scripts, which is the
// same fallback it always had.
func loaderPage(nonce string) []byte {
	page := loaderHTML

	page = strings.ReplaceAll(page, "{{csp}}", cspMeta(nonce))
	page = strings.ReplaceAll(page, "{{nonce}}", nonceAttribute(nonce))
	page = strings.ReplaceAll(page, "{{version}}", versionLine())

	return []byte(page)
}

// cspMeta publishes the nonce to the wasm side, which reads it back for the
// style and script elements it injects once it runs. It is what lets the policy
// name the nonce on both ends of an exchange that goes through the document.
func cspMeta(nonce string) string {
	if nonce == "" {
		return ""
	}
	return `<meta name="csp-nonce" content="` + html.EscapeString(nonce) + `">` + "\n"
}

// nonceAttribute is the attribute the page's one inline script carries.
func nonceAttribute(nonce string) string {
	if nonce == "" {
		return ""
	}
	return ` nonce="` + html.EscapeString(nonce) + `"`
}

// versionLine is the release under the app's name. A build that cannot say
// where it came from shows nothing rather than the linker's defaults.
func versionLine() string {
	v := version.Version
	if v == "" || v == "dev" {
		return ""
	}
	return `<div class="version">` + html.EscapeString(v) + `</div>` + "\n"
}
