package emulation

import (
	"os"
	"path/filepath"
	"testing"
)

func writeROM(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("rom-data"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanROMsMapsFoldersToEmulators(t *testing.T) {
	root := t.TempDir()
	writeROM(t, filepath.Join(root, "ps2", "Shadow of the Colossus.iso"))
	writeROM(t, filepath.Join(root, "switch", "Zelda.nsp"))
	writeROM(t, filepath.Join(root, "snes", "Chrono Trigger.sfc"))
	writeROM(t, filepath.Join(root, "unknown", "ignored.iso"))
	writeROM(t, filepath.Join(root, "ps2", "notes.txt"))

	available := map[string]Emulator{
		"pcsx2":     {ID: "pcsx2", Name: "PCSX2", Binary: "/usr/bin/pcsx2"},
		"ryujinx":   {ID: "ryujinx", Name: "Ryujinx", FlatpakID: "io.github.ryujinx.Ryujinx"},
		"retroarch": {ID: "retroarch", Name: "RetroArch", Binary: "/usr/bin/retroarch"},
	}
	games := scanROMs([]string{root}, available)
	if len(games) != 3 {
		t.Fatalf("esperava 3 ROMs, obtive %d: %+v", len(games), games)
	}

	for _, game := range games {
		switch game.Name {
		case "Shadow of the Colossus":
			if game.Executable != "/usr/bin/pcsx2" {
				t.Fatalf("PS2 executável = %q", game.Executable)
			}
		case "Zelda":
			if game.Executable != "flatpak" || len(game.Arguments) != 3 ||
				game.Arguments[0] != "run" || game.Arguments[1] != "io.github.ryujinx.Ryujinx" {
				t.Fatalf("Switch comando = %q %v", game.Executable, game.Arguments)
			}
		case "Chrono Trigger":
			if game.Executable != "/usr/bin/retroarch" {
				t.Fatalf("SNES executável = %q", game.Executable)
			}
		default:
			t.Fatalf("ROM inesperada: %+v", game)
		}
		if len(game.Platforms) != 1 {
			t.Fatalf("plataformas = %v", game.Platforms)
		}
	}
}

func TestScanROMsAcceptsDirectSystemFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "PS2")
	writeROM(t, filepath.Join(root, "God of War.iso"))
	games := scanROMs([]string{root}, map[string]Emulator{
		"pcsx2": {ID: "pcsx2", Name: "PCSX2", Binary: "pcsx2"},
	})
	if len(games) != 1 || games[0].Name != "God of War" {
		t.Fatalf("jogos = %+v", games)
	}
}

func TestDetectorPrefersNativeThenFlatpak(t *testing.T) {
	detector := Detector{
		LookPath: func(name string) (string, error) {
			if name == "pcsx2" {
				return "/usr/bin/pcsx2", nil
			}
			return "", os.ErrNotExist
		},
		FlatpakInfo: func(id string) bool {
			return id == "org.libretro.RetroArch"
		},
		checkFlatpaks: true,
	}
	available := detector.Detect()
	if available["pcsx2"].Binary != "/usr/bin/pcsx2" {
		t.Fatalf("PCSX2 = %+v", available["pcsx2"])
	}
	if available["retroarch"].FlatpakID != "org.libretro.RetroArch" {
		t.Fatalf("RetroArch = %+v", available["retroarch"])
	}
	if _, ok := available["rpcs3"]; ok {
		t.Fatal("RPCS3 não deveria estar disponível")
	}
}

func TestLaunchCommandReportsMissingEmulator(t *testing.T) {
	provider := NewProvider()
	if _, _, err := provider.LaunchCommand(nil); err == nil {
		t.Fatal("esperava erro para jogo inválido")
	}
}
