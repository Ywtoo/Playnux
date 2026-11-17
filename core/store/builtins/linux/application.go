package linux

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store/builtins/emulation"
)

var clientIDs = []string{
	"steam", "steam-native", "steam-runtime", "com.valvesoftware.steam",
	"heroic", "heroic-games-launcher", "com.heroicgameslauncher.hgl",
	"legendary", "lutris", "net.lutris.lutris", "bottles", "com.usebottles.bottles",
	"gog-galaxy", "galaxyclient", "epicgameslauncher",
	"playnux", "org.playnux.playnux",
	"itch", "itch.io", "io.itch.itch",
	"prismlauncher", "org.prismlauncher.prismlauncher", "minecraft-launcher",
	"gdlauncher", "atlauncher", "technic-launcher", "modrinth", "curseforge",
	"emudeck", "retrodeck", "steam-rom-manager", "protonup-qt",
	"protontricks", "mangohud", "goverlay", "gamescope",
}

var toolWords = []string{
	"launcher", "updater", "installer", "uninstaller", "emulator",
	"retroarch", "protonup", "protontricks", "steam rom manager",
	"playonlinux", "portproton", "faugus", "gamehub",
	// Ferramentas de modding/desenvolvimento que aparecem com categoria Game
	// no menu (ex.: The Witcher Redkit) e não são jogos de verdade.
	"redkit", "modkit", "red kit", "mod kit", "creation kit", "mod manager",
	"mod organizer", "vortex", "wabbajack", "toolkit", "sdk", "editor",
	"benchmark", "dedicated server", "server browser", "game tools",
}

func isLibraryTool(name string) bool {
	name = strings.ToLower(filepath.Base(name))
	for _, suffix := range []string{".desktop", ".appimage", ".sh", ".exe", ".x86_64"} {
		name = strings.TrimSuffix(name, suffix)
	}
	if slices.Contains(clientIDs, name) || emulation.IsEmulatorApplication(name) {
		return true
	}
	for _, word := range toolWords {
		if strings.Contains(name, word) {
			return true
		}
	}
	return false
}

// isToolCategory reconhece entradas de menu marcadas como Game mas que são
// utilitários de desenvolvimento (editores, kits de mod, SDKs).
func isToolCategory(categories []string) bool {
	for _, category := range categories {
		switch strings.ToLower(strings.TrimSpace(category)) {
		case "development", "utility", "settings", "system", "packagemanager":
			return true
		}
	}
	return false
}

// IsImportedLibraryTool also recognizes older scanner entries already stored in the database.
func IsImportedLibraryTool(game *models.Game) bool {
	if game == nil || game.PluginID != "linux" {
		return false
	}
	switch game.Source {
	case "Linux":
		return isLibraryTool(game.GameID) || (len(game.Arguments) == 0 && isLibraryTool(game.Executable))
	case "Flatpak":
		return isLibraryTool(game.GameID)
	case "Local":
		path, ok := strings.CutPrefix(game.GameID, "local:")
		return !ok || isLibraryTool(path) || isStoreLibraryPath(path)
	default:
		return false
	}
}
