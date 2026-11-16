package gameproc

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/playnux/core/models"
)

func writeProcess(t *testing.T, root string, pid int, cmdline []string, env map[string]string, state string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	for _, part := range cmdline {
		raw = append(raw, part...)
		raw = append(raw, 0)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if env != nil {
		raw = raw[:0]
		for key, value := range env {
			raw = append(raw, key+"="+value...)
			raw = append(raw, 0)
		}
		if err := os.WriteFile(filepath.Join(dir, "environ"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte("123 (game) "+state+" 1 2 3"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScannerMatchesInstallDirectoryAndSteamEnv(t *testing.T) {
	root := t.TempDir()
	writeProcess(t, root, 101, []string{"wine", `Z:\home\me\Games\Hades\Hades.exe`}, nil, "S")
	writeProcess(t, root, 202, []string{"steam-launched", "game"}, map[string]string{"SteamAppId": "1091500"}, "S")
	writeProcess(t, root, 303, []string{"zombie", "/home/me/Games/Hades/Hades.exe"}, nil, "Z")

	scanner := &Scanner{ProcRoot: root, Exclude: map[int]bool{101: true}}

	hades := &models.Game{GameID: "1145360", InstallDirectory: "/home/me/Games/Hades"}
	if pid, ok := scanner.Find(hades); ok {
		t.Fatalf("processo 101 deveria estar excluído, encontrei %d", pid)
	}

	scanner.Exclude = nil
	if pid, ok := scanner.Find(hades); !ok || pid != 101 {
		t.Fatalf("Hades: pid=%d ok=%v", pid, ok)
	}
	if scanner.Running(hades) != true {
		t.Fatal("Hades deveria estar em execução")
	}

	cyberpunk := &models.Game{GameID: "1091500", PluginID: "steam", Source: "Steam"}
	if pid, ok := scanner.Find(cyberpunk); !ok || pid != 202 {
		t.Fatalf("Cyberpunk: pid=%d ok=%v", pid, ok)
	}

	missing := &models.Game{GameID: "1", InstallDirectory: "/opt/other"}
	if _, ok := scanner.Find(missing); ok {
		t.Fatal("jogo ausente não deveria casar")
	}
}

func TestMatchRejectsEmptyInstallation(t *testing.T) {
	game := &models.Game{GameID: "42", Source: "GOG"}
	if Match(game, Process{Cmdline: []string{"heroic", "/home/me/Games/Game"}}) {
		t.Fatal("sem install_directory não deve casar por caminho")
	}
}

func TestPathWithin(t *testing.T) {
	cases := []struct {
		candidate string
		dir       string
		want      bool
	}{
		{"/home/me/Games/Hades/Hades.exe", "/home/me/Games/Hades", true},
		{`Z:\home\me\Games\Hades\Hades.exe`, "/home/me/Games/Hades", true},
		{"/home/me/Games/Hades2/Hades2.exe", "/home/me/Games/Hades", false},
		{"/home/me/Games/Hades", "/home/me/Games/Hades/", true},
	}
	for _, tc := range cases {
		if got := pathWithin(tc.candidate, tc.dir); got != tc.want {
			t.Errorf("pathWithin(%q, %q) = %v, quer %v", tc.candidate, tc.dir, got, tc.want)
		}
	}
}
