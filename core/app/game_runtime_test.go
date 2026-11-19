package app

import (
	"testing"

	"github.com/playnux/core/runner/gameproc"
)

func TestFindSteamRuntime(t *testing.T) {
	config := `
		"Steam" {
			"CompatToolMapping" {
				"0" { "name" "proton_experimental" }
				// Per-game selection
				"123" { "name" "GE-Proton10-18" }
				"456" { "name" "proton_hotfix" }
			}
		}`
	if got := findSteamRuntime(config, "123"); got != "GE-Proton10-18" {
		t.Errorf("per-game mapping = %q, want GE-Proton10-18", got)
	}
	if got := findSteamRuntime(config, "789"); got != "" {
		t.Errorf("unknown app mapping = %q, want empty", got)
	}
}

func TestProcessRunner(t *testing.T) {
	tests := []struct {
		name    string
		process gameproc.Process
		want    string
	}{
		{
			name: "Steam Proton path",
			process: gameproc.Process{
				Cmdline: []string{"Hades.exe"},
				Env:     map[string]string{"STEAM_COMPAT_TOOL_PATHS": "/steam/Proton 10.0:/steam/SteamLinuxRuntime_sniper"},
			},
			want: "Proton 10.0",
		},
		{
			name: "Heroic Wine path",
			process: gameproc.Process{
				Cmdline: []string{"/heroic/wine-ge-8-26/bin/wine64", "Hades.exe"},
			},
			want: "wine-ge-8-26",
		},
		{
			name: "unknown Proton version",
			process: gameproc.Process{
				Cmdline: []string{"/usr/bin/wine64", "Hades.exe"},
				Env:     map[string]string{"STEAM_COMPAT_DATA_PATH": "/steam/compatdata/123"},
			},
			want: "Proton · versão não identificada",
		},
		{
			name:    "native game with Proton in the name",
			process: gameproc.Process{Cmdline: []string{"/games/proton-racer"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := processRunner(test.process); got != test.want {
				t.Errorf("runner = %q, want %q", got, test.want)
			}
		})
	}
}
