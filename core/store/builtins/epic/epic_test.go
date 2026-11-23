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

	loginURL := provider.WebLogin().URL
	if !strings.Contains(loginURL, epicClientID) {
		t.Fatalf("URL de login sem client id: %s", loginURL)
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
		if r.URL.Path == "/catalog/api/shared/namespace/store/bulk/items" {
			switch r.URL.Query().Get("id") {
			case "owned-game":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"owned-game": map[string]string{"title": "Resolved Game"},
				})
			case "owned-addon":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"owned-addon": map[string]string{"title": "Extra Map", "itemType": "DLC"},
				})
			default:
				t.Errorf("unexpected catalog item: %q", r.URL.Query().Get("id"))
				http.NotFound(w, r)
			}
			return
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
				{"appName": "aabbccddeeff00112233445566778899", "title": "Third Game"},
				{"appName": "opaque-game-id", "namespace": "store", "catalogItemId": "owned-game"},
				{"appName": "opaque-addon-id", "namespace": "store", "catalogItemId": "owned-addon"},
				{"appName": "aa00112233445566778899aabbccddff"},
				{"catalogItemId": "entitlement-without-app", "title": "Build Interno"},
				{"appName": "MarketplaceProduct", "namespace": "ue", "title": "Marketplace"},
				{"appName": "AlphaPrivate", "sandboxType": "PRIVATE", "title": "Jogo Teste"},
				{"appName": "ExtraMap", "title": "Mapa extra", "categories": []map[string]string{{"path": "mods"}}},
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
	provider.catalogBase = server.URL

	ctx := context.Background()
	if err := provider.Authenticate(ctx, store.StoreAuth{AccessToken: "token-123"}); err != nil {
		t.Fatal(err)
	}

	games, err := provider.FetchLibrary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 4 {
		t.Fatalf("esperava 4 jogos, obtive %d", len(games))
	}
	if atomic.LoadInt32(&pageCalls) != 2 {
		t.Fatalf("paginação não percorreu 2 páginas (%d)", pageCalls)
	}

	byID := map[string]int{}
	for i, game := range games {
		byID[game.GameID] = i
	}
	if i, ok := byID["opaque-game-id"]; !ok || games[i].Name != "Resolved Game" {
		t.Fatal("catalog did not resolve the owned game")
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
