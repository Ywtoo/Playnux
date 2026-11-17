package xbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(err)
	}
}

func TestWebLoginUsesPublicClientAndLoopback(t *testing.T) {
	provider := NewXboxProvider()
	provider.clientID = "test-client"
	provider.redirectURI = "http://127.0.0.1:4242/callback"

	spec := provider.WebLogin()
	if !strings.HasPrefix(spec.URL, defaultAuthorizeBase+"?") {
		t.Fatalf("URL de login inesperada: %s", spec.URL)
	}
	for _, expected := range []string{
		"client_id=test-client",
		"response_type=code",
		"redirect_uri=http%3A%2F%2F127.0.0.1%3A4242%2Fcallback",
		"scope=XboxLive.signin+offline_access",
	} {
		if !strings.Contains(spec.URL, expected) {
			t.Fatalf("URL sem %q: %s", expected, spec.URL)
		}
	}
	if spec.RedirectURLPrefix != provider.redirectURI {
		t.Fatalf("prefixo de redirect = %q", spec.RedirectURLPrefix)
	}
}

func TestExchangeAndRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("client_id") != "test-client" {
			t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "auth-code" {
				t.Errorf("code = %q", r.Form.Get("code"))
			}
			writeJSON(t, w, map[string]any{
				"access_token":  "access-1",
				"refresh_token": "refresh-1",
				"expires_in":    3600,
			})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-1" {
				t.Errorf("refresh_token = %q", r.Form.Get("refresh_token"))
			}
			// A Microsoft pode não devolver um novo refresh token.
			writeJSON(t, w, map[string]any{
				"access_token": "access-2",
				"expires_in":   3600,
			})
		default:
			t.Errorf("grant_type inesperado: %q", r.Form.Get("grant_type"))
		}
	}))
	defer server.Close()

	provider := NewXboxProvider()
	provider.client = server.Client()
	provider.clientID = "test-client"
	provider.tokenBase = server.URL

	ctx := context.Background()
	auth, err := provider.ExchangeCode(ctx, "auth-code")
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken != "access-1" || auth.RefreshToken != "refresh-1" {
		t.Fatalf("tokens = %+v", auth)
	}
	if err := provider.Authenticate(ctx, auth); err != nil {
		t.Fatal(err)
	}

	expired := auth
	expired.AccessToken = "old"
	expired.ExpiresAt = time.Now().Add(-time.Hour)
	if err := provider.Authenticate(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if provider.auth.AccessToken != "access-2" {
		t.Fatalf("access token não renovado: %q", provider.auth.AccessToken)
	}
	if provider.auth.RefreshToken != "refresh-1" {
		t.Fatalf("refresh token deveria ser preservado: %q", provider.auth.RefreshToken)
	}
}

func TestFetchLibraryBuildsChainAndPaginates(t *testing.T) {
	var tokenPageCalls, titleCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "code-1" {
				t.Errorf("form OAuth inesperado: %v", r.Form)
			}
			writeJSON(t, w, map[string]any{
				"access_token":  "msa-token",
				"refresh_token": "msa-refresh",
				"expires_in":    3600,
			})

		case r.URL.Path == "/xbl":
			var payload struct {
				Properties   map[string]string `json:"Properties"`
				RelyingParty string            `json:"RelyingParty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload.Properties["RpsTicket"] != "d=msa-token" {
				t.Errorf("RpsTicket = %q", payload.Properties["RpsTicket"])
			}
			if payload.RelyingParty != "http://auth.xboxlive.com" {
				t.Errorf("RelyingParty XBL = %q", payload.RelyingParty)
			}
			writeJSON(t, w, map[string]any{
				"Token": "xbl-token",
				"DisplayClaims": map[string]any{
					"xui": []map[string]string{{"uhs": "user-hash", "xid": "1234567890"}},
				},
			})

		case r.URL.Path == "/xsts":
			var payload struct {
				Properties map[string]any `json:"Properties"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			users, _ := payload.Properties["UserTokens"].([]any)
			if len(users) != 1 || users[0] != "xbl-token" {
				t.Errorf("UserTokens = %v", payload.Properties["UserTokens"])
			}
			if r.Header.Get("x-xbl-contract-version") != "1" {
				t.Errorf("contrato XBL = %q", r.Header.Get("x-xbl-contract-version"))
			}
			writeJSON(t, w, map[string]any{
				"Token": "xsts-token",
				"DisplayClaims": map[string]any{
					"xui": []map[string]string{{"uhs": "user-hash", "xid": "1234567890"}},
				},
			})

		case strings.HasPrefix(r.URL.Path, "/users/xuid(1234567890)/titles/titlehistory"):
			titleCalls++
			if got := r.Header.Get("Authorization"); got != "XBL3.0 x=user-hash;xsts-token" {
				t.Errorf("authorization = %q", got)
			}
			if r.Header.Get("x-xbl-contract-version") != "2" {
				t.Errorf("contrato titlehub = %q", r.Header.Get("x-xbl-contract-version"))
			}
			if r.URL.Query().Get("maxItems") != "500" {
				t.Errorf("maxItems = %q", r.URL.Query().Get("maxItems"))
			}
			if r.URL.Query().Get("continuationToken") == "" {
				writeJSON(t, w, map[string]any{
					"titles": []map[string]any{
						{
							"titleId": "111",
							"name":    "Halo Infinite",
							"type":    "Game",
							"devices": []string{"Windows.Desktop", "XboxOne"},
							"images":  []map[string]string{{"url": "//images.example/halo.jpg"}},
							"titleHistory": map[string]string{
								"lastTimePlayed": "2026-11-01T12:00:00.0000000Z",
							},
						},
						{
							"titleId": "222",
							"name":    "Forza Horizon 5",
							"type":    "Game",
							"devices": []string{"XboxSeries"},
							"images":  []map[string]string{{"url": "https://images.example/forza.jpg"}},
						},
						{
							"titleId": "333",
							"name":    "Halo DLC",
							"type":    "DLC",
						},
					},
					"pagingInfo": map[string]any{"continuationToken": "next-page", "totalRecords": 4},
				})
				return
			}
			if r.URL.Query().Get("continuationToken") != "next-page" {
				t.Errorf("continuationToken = %q", r.URL.Query().Get("continuationToken"))
			}
			writeJSON(t, w, map[string]any{
				"titles": []map[string]any{
					{
						"titleId": "444",
						"name":    "Ori and the Blind Forest",
						"type":    "Game",
						"devices": []string{"Xbox360", "Windows.Desktop"},
					},
				},
				"pagingInfo": map[string]any{},
			})

		default:
			t.Errorf("endpoint inesperado: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewXboxProvider()
	provider.client = server.Client()
	provider.clientID = "test-client"
	provider.redirectURI = "http://127.0.0.1:1"
	provider.tokenBase = server.URL + "/token"
	provider.xblURL = server.URL + "/xbl"
	provider.xstsURL = server.URL + "/xsts"
	provider.titlehubBase = server.URL

	ctx := context.Background()
	auth, err := provider.ExchangeCode(ctx, "code-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Authenticate(ctx, auth); err != nil {
		t.Fatal(err)
	}

	games, err := provider.FetchLibrary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 3 {
		t.Fatalf("esperava 3 jogos (DLC filtrado), obtive %d", len(games))
	}
	if titleCalls != 2 {
		t.Fatalf("titlehub deveria ter 2 páginas, teve %d chamadas", titleCalls)
	}
	if tokenPageCalls != 0 {
		t.Fatalf("token não deveria ser renovado: %d", tokenPageCalls)
	}
	if provider.AuthSnapshot().AccountID != "1234567890" {
		t.Fatalf("XUID não salvo na sessão: %q", provider.AuthSnapshot().AccountID)
	}

	byID := map[string]*struct {
		name      string
		platforms []string
	}{
		"111": {name: "Halo Infinite", platforms: []string{"PC (Windows)", "Xbox One"}},
		"222": {name: "Forza Horizon 5", platforms: []string{"Xbox Series X|S"}},
		"444": {name: "Ori and the Blind Forest", platforms: []string{"Xbox 360", "PC (Windows)"}},
	}
	for _, game := range games {
		want, ok := byID[game.GameID]
		if !ok {
			t.Fatalf("jogo inesperado: %s", game.GameID)
		}
		if game.Name != want.name {
			t.Errorf("%s: nome = %q", game.GameID, game.Name)
		}
		if strings.Join(game.Platforms, "|") != strings.Join(want.platforms, "|") {
			t.Errorf("%s: plataformas = %v", game.GameID, game.Platforms)
		}
		if game.PluginID != "xbox" || game.Source != "Xbox" {
			t.Errorf("%s: loja = %s/%s", game.GameID, game.PluginID, game.Source)
		}
	}

	halo := games[0]
	if halo.GameID != "111" {
		halo = games[1]
		if halo.GameID != "111" {
			halo = games[2]
		}
	}
	if halo.CoverImage != "https://images.example/halo.jpg" {
		t.Errorf("capa = %q", halo.CoverImage)
	}
	if halo.LastActivity == nil || halo.LastActivity.Year() != 2026 {
		t.Errorf("last activity = %v", halo.LastActivity)
	}

	if _, _, err := provider.LaunchCommand(halo); err == nil {
		t.Fatal("Xbox não deveria lançar nativamente no Linux")
	}
}

func TestXboxStatusErrorMessages(t *testing.T) {
	err := xboxStatusError(http.StatusUnauthorized, []byte(`{"XErr":2148916233}`))
	if !strings.Contains(err.Error(), "perfil Xbox") {
		t.Fatalf("mensagem inesperada: %v", err)
	}
}
