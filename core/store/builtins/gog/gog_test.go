package gog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestFetchLibrarySyncsOwnedGamesAndLocalInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")

	heroicDir := filepath.Join(home, ".config/heroic")
	writeJSON(t, filepath.Join(heroicDir, "store_cache/gog_library.json"), []map[string]any{
		{
			"app_name": "1207658691",
			"title":    "The Witcher 3 (Heroic)",
			"runner":   "gog",
			"extra": map[string]any{
				"images": []map[string]string{
					{"type": "vertical_cover", "url": "//images.gog.com/heroic-cover.jpg"},
				},
			},
		},
	})
	installDir := filepath.Join(home, "Games/Heroic/witcher3")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	uninstaller := filepath.Join(installDir, "unins000.exe")
	if err := os.WriteFile(uninstaller, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(heroicDir, "GamesConfig/1207658691.json"), map[string]any{
		"app_name":   "1207658691",
		"runner":     "wine",
		"winePrefix": "/prefix",
		"wineVersion": map[string]string{
			"bin": "/opt/wine/bin/wine",
		},
		"install": map[string]any{
			"install_path": installDir,
			"executable":   "witcher3.exe",
			"install_size": "5000000",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			query := r.URL.Query()
			if query.Get("client_id") != gogClientID || query.Get("client_secret") != gogClientSecret {
				t.Errorf("credenciais OAuth incorretas: %v", query)
			}
			if query.Get("grant_type") != "authorization_code" || query.Get("code") != "gog-code" {
				t.Errorf("grant inesperado: %v", query)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "gog-token",
				"refresh_token": "gog-refresh",
				"expires_in":    3600,
				"user_id":       "user-42",
			})

		case r.URL.Path == "/user/data/games":
			if r.Header.Get("Authorization") != "Bearer gog-token" {
				t.Errorf("authorization = %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"owned": []int64{1207658691, 998, 999},
			})

		case r.URL.Path == "/account/gameDetails/1207658691.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"title":            "The Witcher 3: Wild Hunt",
				"image":            "//images.gog.com/witcher-cover.jpg",
				"backgroundImage":  "//images.gog.com/witcher-bg.jpg",
				"description":      "RPG de mundo aberto",
				"releaseTimestamp": 1431993600,
				"genres":           []map[string]string{{"name": "RPG"}},
				"developers":       []map[string]string{{"name": "CD PROJEKT RED"}},
			})

		case r.URL.Path == "/account/gameDetails/999.json":
			http.Error(w, "falha simulada", http.StatusInternalServerError)

		case r.URL.Path == "/account/gameDetails/998.json":
			_, _ = w.Write([]byte(`{"title":"Conteúdo extra com título normal"}`))
		case r.URL.Path == "/products/998":
			_, _ = w.Write([]byte(`{"title":"Conteúdo extra com título normal","game_type":"dlc"}`))
		case r.URL.Path == "/products/999":
			_, _ = w.Write([]byte(`{"title":"Nome do catálogo","productType":"GAME"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewGOGProvider()
	provider.SetApplicationCommand(func(id store.ApplicationID, args ...string) (string, []string, bool) {
		if id == store.AppHeroic {
			return "xdg-open", args, true
		}
		return "", nil, false
	})
	provider.client = server.Client()
	provider.authBase = server.URL
	provider.embedBase = server.URL
	provider.productBase = server.URL

	ctx := context.Background()
	auth, err := provider.ExchangeCode(ctx, "gog-code")
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken != "gog-token" || auth.RefreshToken != "gog-refresh" || auth.AccountID != "user-42" {
		t.Fatalf("tokens incorretos: %+v", auth)
	}
	if err := provider.Authenticate(ctx, auth); err != nil {
		t.Fatal(err)
	}

	games, err := provider.FetchLibrary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("esperava 2 jogos, obteve %d", len(games))
	}

	byID := map[string]int{}
	for i, game := range games {
		byID[game.GameID] = i
	}

	witcher := games[byID["1207658691"]]
	if witcher.Name != "The Witcher 3: Wild Hunt" {
		t.Fatalf("nome = %q", witcher.Name)
	}
	if !witcher.IsInstalled || witcher.InstallDirectory != installDir || witcher.InstallSize != 5000000 {
		t.Fatalf("estado local não mesclado: %+v", witcher)
	}
	if witcher.BackgroundImage != "https://images.gog.com/witcher-bg.jpg" {
		t.Fatalf("background = %q", witcher.BackgroundImage)
	}
	if witcher.CoverImage != "https://images.gog.com/witcher-cover.jpg" {
		t.Fatalf("capa = %q", witcher.CoverImage)
	}
	if witcher.ReleaseYear != 2015 {
		t.Fatalf("ano de lançamento = %d", witcher.ReleaseYear)
	}
	if len(witcher.Genres) != 1 || witcher.Developers[0] != "CD PROJEKT RED" {
		t.Fatalf("metadados = %+v", witcher)
	}

	fallback := games[byID["999"]]
	if fallback.Name != "Nome do catálogo" || fallback.IsInstalled {
		t.Fatalf("jogo sem detalhes deveria ter entrada mínima: %+v", fallback)
	}

	cmd, args, err := provider.LaunchCommand(witcher)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "xdg-open" || len(args) != 1 || args[0] != "heroic://launch/gog/1207658691" {
		t.Fatalf("launch inesperado: %s %v", cmd, args)
	}

	cmd, args, err = provider.UninstallCommand(witcher)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "env" {
		t.Fatalf("comando de desinstalação = %s", cmd)
	}
	want := []string{"WINEPREFIX=/prefix", "/opt/wine/bin/wine", uninstaller}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("args de desinstalação = %v, esperava %v", args, want)
	}
}
