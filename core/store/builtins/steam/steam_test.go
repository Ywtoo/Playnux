package steam

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
)

func TestFetchInstalledFindsSecondarySteamLibraries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	primary := filepath.Join(home, ".local/share/Steam/steamapps")
	modern := filepath.Join(home, "External Games")
	legacy := filepath.Join(home, "Other Drive")
	write := func(path, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(primary, "libraryfolders.vdf"), fmt.Sprintf(`
"libraryfolders"
{
	"1"
	{
		"path" %q
		"apps"
		{
			"10" "999"
		}
	}
	"2" %q
	"3" "relative/path"
}
`, modern, legacy))
	write(filepath.Join(modern, "steamapps/appmanifest_10.acf"), "\"name\" \"Modern Game\"\n\"installdir\" \"Modern Game\"\n")
	write(filepath.Join(legacy, "steamapps/appmanifest_20.acf"), "\"name\" \"Legacy Game\"\n\"installdir\" \"Legacy Game\"\n")
	write(filepath.Join(primary, "appmanifest_30.acf"), "\"name\" \"Incomplete\"\n")

	games, err := NewSteamProvider().FetchInstalled(context.Background())
	if err != nil || len(games) != 2 {
		t.Fatalf("jogos importados: %v, erro: %v", games, err)
	}
	want := map[string]string{
		"10": filepath.Join(modern, "steamapps/common/Modern Game"),
		"20": filepath.Join(legacy, "steamapps/common/Legacy Game"),
	}
	for _, game := range games {
		if !game.IsInstalled || game.InstallDirectory != want[game.GameID] {
			t.Errorf("instalação inesperada: %+v", game)
		}
		delete(want, game.GameID)
	}
}

func TestFetchLibraryReadsPublicCommunityGames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/profiles/76561198000000000/games" {
			t.Fatalf("caminho inesperado: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<gamesList>
  <games>
    <game><appID>10</appID><name>Comprado A</name><hoursOnRecord>12.5</hoursOnRecord></game>
    <game><appID>20</appID><name>Comprado B</name></game>
  </games>
</gamesList>`))
	}))
	defer server.Close()

	provider := NewSteamProvider()
	provider.communityBase = server.URL
	if err := provider.Authenticate(context.Background(), store.StoreAuth{AccountID: "76561198000000000"}); err != nil {
		t.Fatal(err)
	}
	games, err := provider.FetchLibrary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("jogos = %+v", games)
	}
	byID := map[string]*models.Game{}
	for _, game := range games {
		byID[game.GameID] = game
	}
	if byID["10"] == nil || byID["20"] == nil {
		t.Fatalf("jogos comprados ausentes: %+v", games)
	}
	if byID["10"].Playtime != 12*3600+30*60 {
		t.Fatalf("playtime = %d", byID["10"].Playtime)
	}
}

func TestIsPlayableGameFiltersProtonAndTools(t *testing.T) {
	cases := []struct {
		appID string
		name  string
		want  bool
	}{
		{"1493710", "Proton Experimental", false},
		{"1628350", "Steam Linux Runtime - Sniper", false},
		{"228980", "Steamworks Common Redistributables", false},
		{"999999", "Some Dedicated Server", false},
		{"888888", "Game SDK Tools", false},
		{"777777", "The Witcher 3: Wild Hunt", true},
		{"666666", "ELDEN RING", true},
	}
	for _, test := range cases {
		if got := IsPlayableGame(test.appID, test.name); got != test.want {
			t.Errorf("IsPlayableGame(%q, %q) = %v, want %v", test.appID, test.name, got, test.want)
		}
	}
}

func TestFetchInstalledSkipsRuntimesAndTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	steamapps := filepath.Join(home, ".local/share/Steam/steamapps")
	write := func(name, manifest string) {
		t.Helper()
		if err := os.MkdirAll(steamapps, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(steamapps, name), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("appmanifest_1493710.acf", "\"name\" \"Proton Experimental\"\n\"installdir\" \"Proton - Experimental\"\n")
	write("appmanifest_228980.acf", "\"name\" \"Steamworks Common Redistributables\"\n\"installdir\" \"Steamworks Shared\"\n")
	write("appmanifest_10.acf", "\"name\" \"Real Game\"\n\"installdir\" \"Real Game\"\n")

	games, err := NewSteamProvider().FetchInstalled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 || games[0].GameID != "10" {
		t.Fatalf("jogos importados = %+v", games)
	}
}

func TestSteamLaunchRequiresClientResolvedByCore(t *testing.T) {
	provider := NewSteamProvider()
	game := &models.Game{GameID: "123"}
	if _, _, err := provider.LaunchCommand(game); err == nil {
		t.Fatal("lançou sem um cliente Steam")
	}
	provider.SetApplicationCommand(func(id store.ApplicationID, args ...string) (string, []string, bool) {
		if id != store.AppSteam || len(args) != 1 || args[0] != "steam://rungameid/123" {
			t.Fatalf("pedido do provedor: %s %v", id, args)
		}
		return "/usr/bin/flatpak", append([]string{"run", "com.valvesoftware.Steam"}, args...), true
	})
	cmd, args, err := provider.LaunchCommand(game)
	if err != nil || cmd != "/usr/bin/flatpak" || len(args) != 3 {
		t.Fatalf("lançamento = %s %v, erro = %v", cmd, args, err)
	}
}
