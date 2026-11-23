package linux

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/playnux/core/models"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path string, elf bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("#!/bin/sh\nexit 0\n")
	if elf {
		content = []byte("\x7fELF\x02\x01\x01\x00rest")
	}
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestProviderReadsDesktopAndFlatpakEntries(t *testing.T) {
	root := t.TempDir()
	desktop := filepath.Join(root, "applications")
	flatpak := filepath.Join(root, "flatpak")
	writeFile(t, filepath.Join(desktop, "hades.desktop"), `[Desktop Entry]
Name=Hades
Comment=Rogue-like
Exec="/opt/Hades/Hades" --fullscreen %U
Icon=hades
Categories=Game;Action;
`)
	writeFile(t, filepath.Join(desktop, "editor.desktop"), `[Desktop Entry]
Name=Editor
Exec=editor %F
Categories=Utility;
`)
	writeFile(t, filepath.Join(desktop, "hidden.desktop"), `[Desktop Entry]
Name=Hidden Game
Exec=hidden
Categories=Game;
NoDisplay=true
`)
	writeFile(t, filepath.Join(flatpak, "org.example.Game.desktop"), `[Desktop Entry]
Name=Flatpak Game
Exec=flatpak run org.example.Game %U
X-Flatpak=org.example.Game
Categories=Game;
`)

	writeFile(t, filepath.Join(desktop, "steam.desktop"), `[Desktop Entry]
Name=Steam
Exec=steam %U
Categories=Game;
`)
	writeFile(t, filepath.Join(flatpak, "org.libretro.RetroArch.desktop"), `[Desktop Entry]
Name=RetroArch
Exec=flatpak run org.libretro.RetroArch
X-Flatpak=org.libretro.RetroArch
Categories=Game;
`)

	provider := &Provider{desktopDirs: []string{desktop}, flatpakDirs: []string{flatpak}}
	provider.SetSelection([]string{"hades", "org.example.Game"})
	games, err := provider.FetchLibrary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("esperava 2 jogos, obtive %d: %+v", len(games), games)
	}

	byName := map[string]bool{}
	for _, game := range games {
		byName[game.Name] = true
		if game.Name == "Hades" {
			if game.Executable != "/opt/Hades/Hades" {
				t.Fatalf("executável = %q", game.Executable)
			}
			if len(game.Arguments) != 1 || game.Arguments[0] != "--fullscreen" {
				t.Fatalf("argumentos = %v", game.Arguments)
			}
			if len(game.Genres) != 1 || game.Genres[0] != "Action" {
				t.Fatalf("gêneros = %v", game.Genres)
			}
		}
		if game.Name == "Flatpak Game" {
			if game.Source != "Flatpak" || game.GameID != "org.example.Game" || !game.IsInstalled {
				t.Fatalf("flatpak = %+v", game)
			}
			command, args, err := provider.LaunchCommand(game)
			if err != nil || command != "flatpak" {
				t.Fatalf("comando = %q %v %v", command, args, err)
			}
			if len(args) != 2 || args[0] != "run" || args[1] != "org.example.Game" {
				t.Fatalf("args flatpak = %v", args)
			}
		}
	}
	if !byName["Hades"] || !byName["Flatpak Game"] {
		t.Fatalf("jogos = %+v", games)
	}
}

func TestScanGameFolderFindsScriptsAndELF(t *testing.T) {
	root := t.TempDir()
	writeExecutable(t, filepath.Join(root, "Hades", "Hades.sh"), false)
	writeExecutable(t, filepath.Join(root, "Celeste", "Celeste"), true)
	writeExecutable(t, filepath.Join(root, "Celeste", "bin", "helper"), true)
	writeFile(t, filepath.Join(root, "Celeste", "readme.txt"), "docs")

	games := scanGameFolder(root)
	if len(games) != 2 {
		t.Fatalf("esperava 2 jogos, obtive %d: %+v", len(games), games)
	}
	for _, game := range games {
		switch game.Name {
		case "Hades":
			if filepath.Base(game.Executable) != "Hades.sh" {
				t.Fatalf("executável de Hades = %s", game.Executable)
			}
		case "Celeste":
			if filepath.Base(game.Executable) != "Celeste" {
				t.Fatalf("executável de Celeste = %s", game.Executable)
			}
		default:
			t.Fatalf("jogo inesperado: %+v", game)
		}
		if !game.IsInstalled || game.PluginID != "linux" {
			t.Fatalf("jogo incompleto: %+v", game)
		}
	}
}

func TestScanIgnoresStoreFoldersAndLaunchers(t *testing.T) {
	root := t.TempDir()
	writeExecutable(t, filepath.Join(root, "Hades", "Hades.sh"), false)
	writeExecutable(t, filepath.Join(root, "SteamLibrary", "steamapps", "common", "Celeste", "Celeste"), true)
	writeExecutable(t, filepath.Join(root, "MyGames", "steamapps", "common", "Hollow Knight", "hollow.sh"), false)

	games := scanGameFolder(root)
	if len(games) != 1 || games[0].Name != "Hades" {
		t.Fatalf("varredura local incluiu pasta de loja: %+v", games)
	}

	desktop := filepath.Join(root, "applications")
	writeFile(t, filepath.Join(desktop, "minecraft-launcher.desktop"), `[Desktop Entry]
Name=Minecraft Launcher
Exec=minecraft-launcher
Categories=Game;
`)
	writeFile(t, filepath.Join(desktop, "hades.desktop"), `[Desktop Entry]
Name=Hades
Exec=hades
Categories=Game;
`)
	provider := &Provider{desktopDirs: []string{desktop}}
	if all := provider.Candidates(); len(all) != 1 || all[0].Name != "Hades" {
		t.Fatalf("candidatos deveriam esconder o launcher: %+v", all)
	}
	provider.SetSelection([]string{"hades"})
	apps, err := provider.FetchLibrary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].Name != "Hades" {
		t.Fatalf("launcher apareceu como jogo: %+v", apps)
	}
}

func TestOldLocalEntriesInsideSteamAreHidden(t *testing.T) {
	game := &models.Game{
		PluginID: "linux",
		Source:   "Local",
		GameID:   "local:/home/me/.local/share/Steam/steamapps/common/Hades/Hades.sh",
	}
	if !IsImportedLibraryTool(game) {
		t.Fatal("entrada local dentro da Steam deveria ser ignorada")
	}
}

func TestSplitExecRemovesPlaceholders(t *testing.T) {
	got := splitExec(`"/opt/My Game/game" --windowed %U %%`)
	want := []string{"/opt/My Game/game", "--windowed", "%"}
	if len(got) != len(want) {
		t.Fatalf("args = %v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("args = %v, quer %v", got, want)
		}
	}
}

func TestLaunchCommandRejectsMissingExecutable(t *testing.T) {
	provider := NewProvider()
	if _, _, err := provider.LaunchCommand(nil); err == nil {
		t.Fatal("esperava erro para jogo inválido")
	}
}
