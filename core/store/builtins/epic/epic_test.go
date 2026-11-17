package epic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/playnux/core/store"
)

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != epicClientID || pass != epicClientSecret {
			t.Errorf("credenciais básicas incorretas")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "auth-code" {
			t.Errorf("form inesperado: %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "token-123",
			"refresh_token": "refresh-123",
			"expires_in":    7200,
			"account_id":    "account-1",
		})
	}))
	defer server.Close()

	provider := NewEpicProvider()
	provider.client = server.Client()
	provider.oauthBase = server.URL

	auth, err := provider.ExchangeCode(context.Background(), "auth-code")
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken != "token-123" || auth.RefreshToken != "refresh-123" {
		t.Fatalf("tokens incorretos: %+v", auth)
	}
	if auth.AccountID != "account-1" {
		t.Fatalf("account id = %q", auth.AccountID)
	}
	if !auth.ExpiresAt.After(time.Now()) {
		t.Fatalf("expiração não calculada: %v", auth.ExpiresAt)
	}

	if err := provider.Authenticate(context.Background(), auth); err != nil {
		t.Fatal(err)
	}
	if !provider.IsAuthenticated() {
		t.Fatal("provider deveria estar autenticado")
	}

	spec := provider.WebLogin()
	if !strings.Contains(spec.URL, epicClientID) {
		t.Fatalf("URL de login sem client id: %s", spec.URL)
	}
	if spec.RedirectURLPrefix == "" || len(spec.CookieNames) == 0 {
		t.Fatalf("spec de login incompleta: %+v", spec)
	}
}

func TestFetchLibraryMergesCloudAndLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "") // força ausência de legendary/heroic binários

	heroicDir := filepath.Join(home, ".config/heroic")
	writeJSON(t, filepath.Join(heroicDir, "store_cache/legendary_library.json"), []map[string]any{
		{
			"app_name": "Sugar",
			"title":    "Sugar",
			"extra": map[string]any{
				"images": []map[string]string{
					{"type": "DieselGameBoxTall", "url": "https://cdn.example/sugar.jpg"},
				},
			},
		},
	})
	writeJSON(t, filepath.Join(heroicDir, "GamesConfig/Sugar.json"), map[string]any{
		"app_name": "Sugar",
		"runner":   "legendary",
		"install": map[string]any{
			"install_path": "/games/sugar",
			"executable":   "sugar.exe",
			"install_size": "2048",
			"platform":     "Windows",
		},
	})

	var pageCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "bearer token-123" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/library/api/public/items" {
			t.Errorf("path inesperado: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		cursor := r.URL.Query().Get("cursor")
		if cursor == "" {
			atomic.AddInt32(&pageCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"records": []map[string]any{
					{
						"appName":     "Sugar",
						"productName": "Sugar",
						"metadata": map[string]any{
							"title":     "Sugar",
							"keyImages": []map[string]string{{"type": "DieselGameBoxTall", "url": "https://cdn.example/sugar-cloud.jpg"}},
						},
					},
					{
						"appName":     "CloudGame",
						"productName": "Cloud Game",
						"metadata": map[string]any{
							"title":       "Cloud Game",
							"description": "Descrição",
							"developer":   "Cloud Studio",
							"keyImages": []map[string]string{
								{"type": "OfferImageTall", "url": "https://cdn.example/cloud-cover.jpg"},
								{"type": "DieselStoreFrontWide", "url": "https://cdn.example/cloud-bg.jpg"},
							},
						},
					},
				},
				"responseMetadata": map[string]any{"nextCursor": "page-2"},
			})
			return
		}

		if cursor != "page-2" {
			t.Errorf("cursor inesperado: %q", cursor)
		}
		atomic.AddInt32(&pageCalls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"records": []map[string]any{
				{"appName": "ThirdGame", "productName": "Third Game"},
			},
			"responseMetadata": map[string]any{},
		})
	}))
	defer server.Close()

	provider := NewEpicProvider()
	provider.SetApplicationCommand(func(id store.ApplicationID, args ...string) (string, []string, bool) {
		if id == store.AppHeroic {
			return "xdg-open", args, true
		}
		return "", nil, false
	})
	provider.client = server.Client()
	provider.libraryBase = server.URL

	ctx := context.Background()
	if err := provider.Authenticate(ctx, store.StoreAuth{AccessToken: "token-123"}); err != nil {
		t.Fatal(err)
	}

	games, err := provider.FetchLibrary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 3 {
		t.Fatalf("esperava 3 jogos, obtive %d", len(games))
	}
	if atomic.LoadInt32(&pageCalls) != 2 {
		t.Fatalf("paginação não percorreu 2 páginas (%d)", pageCalls)
	}

	byID := map[string]int{}
	for i, game := range games {
		byID[game.GameID] = i
	}

	sugar := games[byID["Sugar"]]
	if !sugar.IsInstalled || sugar.InstallDirectory != "/games/sugar" || sugar.InstallSize != 2048 {
		t.Fatalf("estado local do Sugar não mesclado: %+v", sugar)
	}
	if sugar.PluginID != "epic" || sugar.Source != "Epic" {
		t.Fatalf("metadados de loja incorretos: %+v", sugar)
	}

	cloud := games[byID["CloudGame"]]
	if cloud.Name != "Cloud Game" || cloud.Description != "Descrição" {
		t.Fatalf("metadados da nuvem incorretos: %+v", cloud)
	}
	if cloud.CoverImage != "https://cdn.example/cloud-cover.jpg" {
		t.Fatalf("capa = %q", cloud.CoverImage)
	}
	if cloud.BackgroundImage != "https://cdn.example/cloud-bg.jpg" {
		t.Fatalf("background = %q", cloud.BackgroundImage)
	}
	if len(cloud.Developers) != 1 || cloud.Developers[0] != "Cloud Studio" {
		t.Fatalf("desenvolvedores = %v", cloud.Developers)
	}

	cmd, args, err := provider.LaunchCommand(cloud)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "xdg-open" || len(args) != 1 || args[0] != "heroic://launch/legendary/CloudGame" {
		t.Fatalf("launch inesperado: %s %v", cmd, args)
	}

	if _, _, err := provider.UninstallCommand(cloud); err == nil {
		t.Fatal("sem Legendary a desinstalação deveria retornar erro explicativo")
	}
}
