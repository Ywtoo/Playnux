// Package applications resolves the external clients approved by the Core.
package applications

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/playnux/core/store"
)

type application struct {
	name     string
	binaries []string
	flatpak  string
	scheme   string
}

var catalog = map[store.ApplicationID]application{
	store.AppSteam:     {"Steam", []string{"steam"}, "com.valvesoftware.Steam", "steam"},
	store.AppHeroic:    {"Heroic", []string{"heroic", "heroic-games-launcher"}, "com.heroicgameslauncher.hgl", "heroic"},
	store.AppLegendary: {"Legendary", []string{"legendary"}, "", ""},
	store.AppWine:      {"Wine", []string{"wine"}, "", ""},
}

func Name(id store.ApplicationID) string {
	return catalog[id].name
}

func Command(id store.ApplicationID, args ...string) (string, []string, bool) {
	app, ok := catalog[id]
	if !ok {
		return "", nil, false
	}
	for _, binary := range app.binaries {
		if command, err := exec.LookPath(binary); err == nil {
			return command, args, true
		}
	}
	if app.flatpak != "" {
		if flatpak, err := exec.LookPath("flatpak"); err == nil && installed(flatpak, "info", app.flatpak) {
			return flatpak, append([]string{"run", app.flatpak}, args...), true
		}
	}
	// An AppImage or a distribution package can register a URI handler without a PATH entry.
	if app.scheme != "" && hasURIHandler(app.scheme) &&
		(len(args) == 0 || (len(args) == 1 && strings.HasPrefix(args[0], app.scheme+"://"))) {
		if opener, err := exec.LookPath("xdg-open"); err == nil {
			return opener, args, true
		}
	}
	return "", nil, false
}

func installed(command string, args ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	return exec.CommandContext(ctx, command, args...).Run() == nil
}

func hasURIHandler(scheme string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	output, err := exec.CommandContext(ctx, "xdg-mime", "query", "default", "x-scheme-handler/"+scheme).Output()
	id := strings.TrimSpace(string(output))
	if err != nil || filepath.Base(id) != id || !strings.HasSuffix(id, ".desktop") {
		return false
	}
	dirs := filepath.SplitList(os.Getenv("XDG_DATA_DIRS"))
	if len(dirs) == 0 {
		dirs = []string{"/usr/local/share", "/usr/share"}
	}
	userData := os.Getenv("XDG_DATA_HOME")
	if userData == "" {
		if home, err := os.UserHomeDir(); err == nil {
			userData = filepath.Join(home, ".local", "share")
		}
	}
	if userData != "" {
		dirs = append([]string{userData}, dirs...)
	}
	for _, dir := range dirs {
		if entry, err := os.Stat(filepath.Join(dir, "applications", id)); err == nil && entry.Mode().IsRegular() {
			return true
		}
	}
	return false
}
