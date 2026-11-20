package loginbridge

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestReportScriptEmbedsLoginState(t *testing.T) {
	script := ReportScript(Options{
		Token:       `tok"en`,
		ReportURL:   "http://127.0.0.1:9999/report",
		RedirectURL: "https://www.epicgames.com/id/api/redirect",
		Cookies:     []string{"EPIC_BEARER_TOKEN", "exchange_code"},
	})

	for _, want := range []string{
		`"tok\"en"`,
		"http://127.0.0.1:9999/report",
		"https://www.epicgames.com/id/api/redirect",
		`["EPIC_BEARER_TOKEN","exchange_code"]`,
		"document.cookie",
		"authorizationCode",
		"location.href",
		"window.open =",
		"a[target='_blank']",
		"followPopup",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script não contém %q", want)
		}
	}
}

func TestDecodeReport(t *testing.T) {
	report := Report{
		URL:     "https://example.com/callback?code=abc",
		Cookies: map[string]string{"code": "abc"},
		Body:    `{"authorizationCode":"abc"}`,
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)

	decoded, err := DecodeReport(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.URL != report.URL || decoded.Cookies["code"] != "abc" || decoded.Body != report.Body {
		t.Fatalf("payload decodificado = %+v", decoded)
	}

	if _, err := DecodeReport("não-base64"); err == nil {
		t.Fatal("esperava erro para base64 inválido")
	}
}
