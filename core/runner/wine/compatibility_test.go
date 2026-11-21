package wine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/launcher"
)

func TestWindowsExecutablePrefersInstalledExecutable(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"unins000.exe", "Game.exe"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("exe"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	game := &models.Game{PluginID: "steam", InstallDirectory: root}
	if got := WindowsExecutable(game); got != filepath.Join(root, "Game.exe") {
		t.Fatalf("executable = %q", got)
	}
}

func TestApplyBuildsIsolatedProtonCommand(t *testing.T) {
	root := t.TempDir()
	proton := filepath.Join(root, "GE-Proton9", "proton")
	if err := os.MkdirAll(filepath.Dir(proton), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proton, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "Game.exe")
	if err := os.WriteFile(exe, []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}

	game := &models.Game{ID: "game-1", PluginID: "steam", InstallDirectory: root}
	options := &launcher.Options{Runner: "proton", ProtonPath: proton, Arguments: []string{"--windowed"}}
	dataDir := filepath.Join(root, "data")
	if err := Apply(options, game, dataDir); err != nil {
		t.Fatal(err)
	}
	if options.Command != proton {
		t.Fatalf("command = %q", options.Command)
	}
	if len(options.CommandArgs) != 2 || options.CommandArgs[0] != "run" || options.CommandArgs[1] != exe {
		t.Fatalf("arguments = %v", options.CommandArgs)
	}
	if options.Environment["STEAM_COMPAT_DATA_PATH"] == "" || options.Environment["STEAM_COMPAT_CLIENT_INSTALL_PATH"] == "" {
		t.Fatalf("environment = %v", options.Environment)
	}
	if options.Directory != root {
		t.Fatalf("directory = %q", options.Directory)
	}
}

func TestApplyKeepsNativeLauncherByDefault(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Game.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}
	game := &models.Game{ID: "game-2", PluginID: "gog", InstallDirectory: root}
	options := &launcher.Options{Runner: "auto"}
	if err := Apply(options, game, filepath.Join(root, "data")); err != nil {
		t.Fatal(err)
	}
	if options.Command != "" {
		t.Fatalf("store game should use its native launcher, got %q", options.Command)
	}
}
