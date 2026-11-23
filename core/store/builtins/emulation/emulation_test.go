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

func TestScanCatalogMapsFoldersToProfiles(t *testing.T) {
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
	games := scanCatalog([]string{root}, nil, Catalog(available, nil))
	if len(games) != 3 {
		t.Fatalf("expected 3 ROMs, got %d: %+v", len(games), games)
	}

	for _, game := range games {
		switch game.Name {
		case "Shadow of the Colossus":
			if game.Executable != "/usr/bin/pcsx2" || len(game.Arguments) < 2 {
				t.Fatalf("PS2 command = %q %v", game.Executable, game.Arguments)
			}
		case "Zelda":
			if game.Executable != "flatpak" || len(game.Arguments) < 3 ||
				game.Arguments[0] != "run" || game.Arguments[1] != "io.github.ryujinx.Ryujinx" {
				t.Fatalf("Switch command = %q %v", game.Executable, game.Arguments)
			}
		case "Chrono Trigger":
			if game.Executable != "/usr/bin/retroarch" {
				t.Fatalf("SNES executable = %q", game.Executable)
			}
		default:
			t.Fatalf("unexpected ROM: %+v", game)
		}
		if len(game.Platforms) != 1 {
			t.Fatalf("platforms = %v", game.Platforms)
		}
	}
}

func TestScanCatalogAcceptsDirectSystemFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "PS2")
	writeROM(t, filepath.Join(root, "God of War.iso"))
	catalog := Catalog(map[string]Emulator{
		"pcsx2": {ID: "pcsx2", Name: "PCSX2", Binary: "pcsx2"},
	}, nil)
	games := scanCatalog([]string{root}, nil, catalog)
	if len(games) != 1 || games[0].Name != "God of War" {
		t.Fatalf("games = %+v", games)
	}
}

func TestMappedFolderUsesChosenCustomProfile(t *testing.T) {
	root := t.TempDir()
	writeROM(t, filepath.Join(root, "whatever", "Custom Game.bin"))

	custom := EmulatorDefinition{
		ID:         "myemu",
		Name:       "My Emulator",
		Executable: "/usr/bin/myemu",
		Profiles: []EmulatorProfile{{
			ProfileID:    "myemu:custom",
			Name:         "My Emulator - Custom",
			Platforms:    []string{"Custom System"},
			Extensions:   []string{"bin"},
			ArgsTemplate: `--rom "%ROM%" --fullscreen`,
		}},
	}
	catalog := Catalog(nil, []EmulatorDefinition{custom})
	games := scanCatalog([]string{root}, map[string]string{root: "myemu:custom"}, catalog)
	if len(games) != 1 {
		t.Fatalf("games = %+v", games)
	}
	got := games[0]
	if got.Executable != "/usr/bin/myemu" {
		t.Fatalf("executable = %q", got.Executable)
	}
	wantArgs := []string{"--rom", filepath.Join(root, "whatever", "Custom Game.bin"), "--fullscreen"}
	if len(got.Arguments) != len(wantArgs) {
		t.Fatalf("arguments = %v", got.Arguments)
	}
	for index := range wantArgs {
		if got.Arguments[index] != wantArgs[index] {
			t.Fatalf("arguments = %v, want %v", got.Arguments, wantArgs)
		}
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
		t.Fatal("RPCS3 should not be available")
	}
}

func TestNormalizeCustomValidatesProfiles(t *testing.T) {
	if _, err := NormalizeCustom(EmulatorDefinition{ID: "x"}); err == nil {
		t.Fatal("expected an error for a custom emulator without executable")
	}
	def, err := NormalizeCustom(EmulatorDefinition{
		ID:         "My Emu",
		Name:       "My Emulator",
		Executable: "/usr/bin/myemu",
		Profiles: []EmulatorProfile{{
			Name:       "Fast",
			Platforms:  []string{"PlayStation 2"},
			Extensions: []string{"iso", ".CHD"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if def.ID != "myemu" || def.Profiles[0].ProfileID != "myemu:1" {
		t.Fatalf("normalized = %+v", def)
	}
	if len(def.Profiles[0].Extensions) != 2 || def.Profiles[0].Extensions[0] != ".iso" {
		t.Fatalf("extensions = %v", def.Profiles[0].Extensions)
	}
	if def.Profiles[0].ArgsTemplate != `"{ImagePath}"` {
		t.Fatalf("default template = %q", def.Profiles[0].ArgsTemplate)
	}
}

func TestLaunchCommandReportsMissingEmulator(t *testing.T) {
	provider := NewProvider()
	if _, _, err := provider.LaunchCommand(nil); err == nil {
		t.Fatal("expected an error for an invalid game")
	}
}

func TestScanCatalogRespectsProfileOnNestedFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ps2")
	manual := filepath.Join(root, "coleção")
	writeROM(t, filepath.Join(root, "PS2 Game.iso"))
	writeROM(t, filepath.Join(manual, "Manual Game.iso"))
	writeROM(t, filepath.Join(manual, "Different Extension.chd"))

	catalog := Catalog(
		map[string]Emulator{"pcsx2": {ID: "pcsx2", Name: "PCSX2", Binary: "pcsx2"}},
		[]EmulatorDefinition{{
			ID: "manual", Name: "Manual", Executable: "/usr/bin/manual",
			Profiles: []EmulatorProfile{{ProfileID: "manual:iso", Extensions: []string{"iso"}}},
		}},
	)
	games := scanCatalog([]string{root, manual}, map[string]string{manual: "manual:iso"}, catalog)
	if len(games) != 2 || games[0].Executable != "/usr/bin/manual" || games[1].Executable != "pcsx2" {
		for _, game := range games {
			t.Logf("%s: %s", game.Name, game.Executable)
		}
		t.Fatalf("esperava dois jogos com os perfis das respectivas pastas, obtive %d", len(games))
	}
}
