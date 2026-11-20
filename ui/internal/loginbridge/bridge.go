// Package loginbridge constrói o script injetado na janela de login.
//
// A página remota (Steam/Epic/GOG/Xbox) não tem acesso aos bindings do Wails,
// porque os bindings usam fetch para a origem do app. Por isso o script detecta
// o fim do fluxo, serializa URL/cookies/corpo e navega para um servidor local
// efêmero. A navegação de topo não sofre as restrições de CORS/CSP de um fetch.
package loginbridge

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
)

// Options descreve uma sessão de login.
type Options struct {
	Token       string
	ReportURL   string
	RedirectURL string
	Cookies     []string
}

// ReportScript devolve o JavaScript injetado em cada carregamento da WebView.
func ReportScript(options Options) string {
	cookieNames, _ := json.Marshal(options.Cookies)
	return fmt.Sprintf(`(function () {
  if (window.__playnuxLoginHook) { return; }
  window.__playnuxLoginHook = true;	var token = %s;
  var reportURL = %s;
  var redirectPrefix = %s;
  var cookieNames = %s;

  // O login da Epic oferece "entrar com Steam" numa janela popup. A WebView do
  // Wails não cria popups, então o clique parecia não fazer nada. Converte o
  // popup para a mesma janela; o redirect final continua sendo observado abaixo.
  function followPopup(url) {
    try {
      var target = new URL(url, location.href);
      if (target.protocol !== "http:" && target.protocol !== "https:") { return false; }
      setTimeout(function () { location.assign(target.href); }, 0);
      return true;
    } catch (e) {
      return false;
    }
  }

  function popupStub(url) {
    return {
      closed: false,
      focus: function () {},
      close: function () {},
      postMessage: function () {},
      location: { href: url }
    };
  }

  if (!window.__playnuxPopupHook) {
    window.__playnuxPopupHook = true;
    window.open = function (url) {
      if (!url) { return null; }
      return followPopup(url) ? popupStub(String(url)) : null;
    };
    document.addEventListener("click", function (event) {
      var anchor = event.target && event.target.closest ? event.target.closest("a[target='_blank']") : null;
      if (!anchor || !anchor.href) { return; }
      if (!followPopup(anchor.href)) { return; }
      event.preventDefault();
      event.stopPropagation();
    }, true);
  }


  function readCookies() {
    var result = {};
    try {
      var raw = document.cookie || "";
      var parts = raw.split(";");
      for (var i = 0; i < parts.length; i++) {
        var item = parts[i].trim();
        if (!item) { continue; }
        var idx = item.indexOf("=");
        if (idx < 0) { continue; }
        result[item.slice(0, idx)] = item.slice(idx + 1);
      }
    } catch (e) {}
    return result;
  }

  function bodyText() {
    try {
      return (document.body && document.body.innerText) ? document.body.innerText : "";
    } catch (e) {
      return "";
    }
  }

  function base64url(value) {
    var bytes = new TextEncoder().encode(value);
    var binary = "";
    for (var i = 0; i < bytes.length; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }

  function finished(cookies) {
    for (var i = 0; i < cookieNames.length; i++) {
      if (cookies[cookieNames[i]]) { return true; }
    }
    var href = location.href;
    if (/[?&#](code|authorizationCode|exchange_code)=/.test(href)) { return true; }
    if (redirectPrefix && href.indexOf(redirectPrefix) === 0) {
      // Alguns provedores devolvem o código no JSON da página de redirect.
      var text = bodyText();
      if (/"(authorizationCode|exchange_code)"\s*:/.test(text)) { return true; }
    }
    return false;
  }

  function report() {
    var cookies = readCookies();
    if (!finished(cookies)) { return; }
    var payload = {
      url: location.href,
      cookies: cookies,
      body: bodyText().slice(0, 32768)
    };
    var target = reportURL + "?t=" + encodeURIComponent(token) +
      "&d=" + base64url(JSON.stringify(payload));
    clearInterval(timer);
    location.href = target;
  }

  var timer = setInterval(report, 250);
  report();
})();`, jsString(options.Token), jsString(options.ReportURL),
		jsString(options.RedirectURL), string(cookieNames))
}

// Report é o payload que a página remota envia ao servidor local.
type Report struct {
	URL     string            `json:"url"`
	Cookies map[string]string `json:"cookies"`
	Body    string            `json:"body"`
}

// DecodeReport decodifica o parâmetro "d" (base64url sem padding) gerado pelo
// script injetado.
func DecodeReport(encoded string) (Report, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Report{}, fmt.Errorf("payload de login inválido: %w", err)
	}
	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return Report{}, fmt.Errorf("payload de login inválido: %w", err)
	}
	if report.Cookies == nil {
		report.Cookies = map[string]string{}
	}
	return report, nil
}

// jsString serializa uma string Go como literal JavaScript seguro.
func jsString(value string) string {
	return strconv.Quote(value)
}
