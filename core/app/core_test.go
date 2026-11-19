package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store/builtins/emulation"
	"github.com/playnux/core/store/builtins/linux"
)

func writeLocalGame(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "Hades")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Hades.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func findGame(games []*models.Game, name, source string) *models.Game {
	for _, game := range games {
		if game.Name == name && game.Source == source {
			return game
		}
	}
	return nil
}

func TestLocalFolderAddsAndRemovesGames(t *testing.T) {
	root := t.TempDir()
	writeLocalGame(t, root)

	core, err := NewWithProviders(":memory:", nil, nil, linux.NewProvider(), emulation.NewProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()

	ctx := context.Background()
	if _, err := core.AddLocalFolder(ctx, root); err != nil {
		t.Fatal(err)
	}
	folders := core.LocalFolders()
	if len(folders) != 1 || folders[0] != root {
		t.Fatalf("pastas = %v", folders)
	}

	games, err := core.ListGames(ctx, "all")
	if err != nil {
		t.Fatal(err)
	}
	hades := findGame(games, "Hades", "Local")
	if hades == nil {
		t.Fatalf("jogo local não importado: %+v", games)
	}

	if _, err := core.RemoveLocalFolder(ctx, root); err != nil {
		t.Fatal(err)
	}
	games, err = core.ListGames(ctx, "all")
	if err != nil {
		t.Fatal(err)
	}
	if findGame(games, "Hades", "Local") != nil {
		t.Fatalf("jogo local deveria ter sido removido: %+v", games)
	}
}

func TestRomFolderAddsGames(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ps2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Shadow of the Colossus.iso"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}

	core, err := NewWithProviders(":memory:", nil, nil, linux.NewProvider(), emulation.NewProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()

	ctx := context.Background()
	if err := core.SaveEmulator(ctx, emulation.CustomEmulator{
		ID:         "pcsx2-test",
		Name:       "PCSX2 Test",
		Executable: "/usr/bin/pcsx2",
		Profiles: []emulation.EmulatorProfile{{
			ProfileID:    "pcsx2-test:ps2",
			Name:         "PCSX2 - Fast Boot",
			Platforms:    []string{"PlayStation 2"},
			Extensions:   []string{"iso"},
			ArgsTemplate: `--fullscreen --batch "{ImagePath}"`,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := core.AddEmulationFolder(ctx, root, "pcsx2-test:ps2"); err != nil {
		t.Fatal(err)
	}
	games, err := core.ListGames(ctx, "all")
	if err != nil {
		t.Fatal(err)
	}
	rom := findGame(games, "Shadow of the Colossus", "Emulação")
	if rom == nil {
		t.Fatalf("ROM não importada: %+v", games)
	}

	if _, err := core.RemoveRomFolder(ctx, root); err != nil {
		t.Fatal(err)
	}
	games, err = core.ListGames(ctx, "all")
	if err != nil {
		t.Fatal(err)
	}
	if findGame(games, "Shadow of the Colossus", "Emulação") != nil {
		t.Fatalf("ROM deveria ter sido removida: %+v", games)
	}
}

func TestLibraryHidesPreviouslyImportedTools(t *testing.T) {
	ctx := context.Background()
	core, err := NewWithProviders(":memory:", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	if err := core.db.UpsertGames(ctx, []*models.Game{
		{GameID: "com.valvesoftware.Steam", Source: "Flatpak", PluginID: "linux", Name: "Steam"},
		{GameID: "org.libretro.RetroArch", Source: "Flatpak", PluginID: "linux", Name: "RetroArch"},
		{GameID: "rom:/games/example.sfc", Source: "Emulação", PluginID: "emulation", Name: "Example ROM", Executable: "/usr/bin/retroarch"},
		{GameID: "123", Source: "Steam", PluginID: "steam", Name: "Example Game"},
	}); err != nil {
		t.Fatal(err)
	}
	games, err := core.ListGames(ctx, "all")
	if err != nil || len(games) != 2 {
		t.Fatalf("biblioteca = %v, erro = %v", games, err)
	}
	if games, err := core.SearchGames(ctx, "RetroArch"); err != nil || len(games) != 0 {
		t.Fatalf("busca = %v, erro = %v", games, err)
	}
}

func TestLaunchOptionsPersistPerLocalGame(t *testing.T) {
	ctx := context.Background()
	core, err := NewWithProviders(":memory:", nil, nil, linux.NewProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	local := &models.Game{GameID: "local", Source: "Manual", PluginID: "linux", Name: "Local Game"}
	remote := &models.Game{GameID: "42", Source: "Steam", PluginID: "steam", Name: "Steam Game"}
	if err := core.db.UpsertGames(ctx, []*models.Game{local, remote}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := core.SaveLaunchOptions(ctx, local.ID, dir, []string{"--windowed", "one argument"}, "auto", ""); err != nil {
		t.Fatal(err)
	}
	options, err := core.LaunchOptions(ctx, local)
	if err != nil || options.Directory != dir || len(options.Arguments) != 2 || options.Arguments[1] != "one argument" {
		t.Fatalf("opções = %+v, erro = %v", options, err)
	}
	if err := core.SaveLaunchOptions(ctx, remote.ID, dir, nil, "auto", ""); err != nil {
		t.Fatalf("jogos de loja PC também aceitam opções próprias: %v", err)
	}
	remoteOptions, err := core.LaunchOptions(ctx, remote)
	if err != nil || remoteOptions.Directory != dir {
		t.Fatalf("opções Steam = %+v, erro = %v", remoteOptions, err)
	}
}
