package heroic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

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

func TestEpicLibraryMergesCacheGamesConfigAndLegendary(t *testing.T) {
	home := setHome(t)
	heroicDir := filepath.Join(home, ".config/heroic")

	writeJSON(t, filepath.Join(heroicDir, "store_cache/legendary_library.json"), []map[string]any{
		{
			"app_name": "Sugar",
			"title":    "Sugar",
			"runner":   "legendary",
			"extra": map[string]any{
				"images": []map[string]string{
					{"type": "DieselGameBoxTall", "url": "https://cdn.example/cover.jpg"},
					{"type": "DieselGameBox", "url": "https://cdn.example/background.jpg"},
				},
			},
		},
	})
	writeJSON(t, filepath.Join(heroicDir, "GamesConfig/Sugar.json"), map[string]any{
		"app_name":   "Sugar",
		"runner":     "legendary",
		"winePrefix": "/prefix",
		"wineVersion": map[string]string{
			"bin": "/opt/wine/bin/wine",
		},
		"install": map[string]any{
			"install_path": "/games/sugar",
			"executable":   "sugar.exe",
			"install_size": "123456",
			"platform":     "Windows",
		},
	})
	// GamesConfig é compartilhado: um jogo GOG não pode vazar para a Epic.
	writeJSON(t, filepath.Join(heroicDir, "GamesConfig/1207658691.json"), map[string]any{
		"app_name": "1207658691",
		"runner":   "gog",
		"install": map[string]any{
			"install_path": "/games/witcher",
			"executable":   "witcher.exe",
			"platform":     "Windows",
		},
	})
	writeJSON(t, filepath.Join(home, ".config/legendary/installed.json"), map[string]any{
		"CloudGame": map[string]any{
			"title":        "Cloud Game",
			"install_path": "/games/cloud",
			"executable":   "cloud.exe",
			"install_size": 999,
		},
	})

	entries := EpicLibrary()
	if len(entries) != 2 {
		t.Fatalf("esperava 2 jogos Epic (sem vazar GOG), obtive %d", len(entries))
	}
	for _, entry := range entries {
		if entry.AppName == "1207658691" {
			t.Fatal("jogo GOG vazou para a biblioteca Epic")
		}
	}

	byName := map[string]*Entry{}
	for _, entry := range entries {
		byName[entry.AppName] = entry
	}

	sugar := byName["Sugar"]
	if sugar == nil {
		t.Fatal("Sugar não encontrado")
	}
	if !sugar.Installed() || sugar.Install.Path != "/games/sugar" {
		t.Fatalf("install do Sugar incorreto: %+v", sugar.Install)
	}
	if sugar.Install.Size != 123456 {
		t.Fatalf("install_size deveria aceitar string, obtive %d", sugar.Install.Size)
	}
	if sugar.Install.WineBinary != "/opt/wine/bin/wine" || sugar.Install.WinePrefix != "/prefix" {
		t.Fatalf("config de Wine não lida: %+v", sugar.Install)
	}
	if got := sugar.Install.ExecutablePath(); got != "/games/sugar/sugar.exe" {
		t.Fatalf("ExecutablePath = %q", got)
	}
	if sugar.Cover() != "https://cdn.example/cover.jpg" {
		t.Fatalf("capa = %q", sugar.Cover())
	}
	if sugar.Background() != "https://cdn.example/background.jpg" {
		t.Fatalf("background = %q", sugar.Background())
	}

	cloud := byName["CloudGame"]
	if cloud == nil || !cloud.Installed() {
		t.Fatalf("jogo instalado pelo Legendary não lido: %+v", cloud)
	}
	if cloud.Title != "Cloud Game" {
		t.Fatalf("título = %q", cloud.Title)
	}
}

func TestGOGLibraryNormalizesImages(t *testing.T) {
	home := setHome(t)
	writeJSON(t, filepath.Join(home, ".config/heroic/store_cache/gog_library.json"), []map[string]any{
		{
			"app_name": "1207658691",
			"title":    "The Witcher 3",
			"runner":   "gog",
			"extra": map[string]any{
				"images": []map[string]string{
					{"type": "vertical_cover", "url": "//images.gog.com/cover.jpg"},
					{"type": "background", "url": "//images.gog.com/background.jpg"},
				},
			},
		},
	})

	entries := GOGLibrary()
	if len(entries) != 1 {
		t.Fatalf("esperava 1 jogo, obtive %d", len(entries))
	}

	entry := entries[0]
	if entry.Cover() != "https://images.gog.com/cover.jpg" {
		t.Fatalf("capa = %q", entry.Cover())
	}
	if entry.Background() != "https://images.gog.com/background.jpg" {
		t.Fatalf("background = %q", entry.Background())
	}
	if entry.Installed() {
		t.Fatal("jogo sem install_path não deveria estar instalado")
	}
}
