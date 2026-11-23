package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/gameproc"
	"github.com/playnux/core/store/builtins/heroic"
	"github.com/playnux/core/store/builtins/steam"
)

type GameRuntime struct {
	Configured string `json:"configured"`
	Automatic  string `json:"automatic"`
	Running    string `json:"running"`
	LastUsed   string `json:"last_used,omitempty"`
}

func (c *App) GameRuntime(ctx context.Context, id string) (GameRuntime, error) {
	game, err := c.GetGame(ctx, id)
	if err != nil {
		return GameRuntime{}, err
	}
	automatic := automaticRunner(game)
	result := GameRuntime{Configured: c.configuredRunner(ctx, game, automatic), Automatic: automatic}
	if game.PluginID == "steam" {
		result.LastUsed = lastSteamProton(game)
	}
	for _, process := range gameproc.NewScanner().List() {
		if !process.Zombie && gameproc.Match(game, process) {
			if name := processRunner(process); name != "" {
				result.Running = name
				break
			}
		}
	}
	return result, nil
}

func configuredRuntime(game *models.Game) string {
	switch game.PluginID {
	case "steam":
		if name := steamRuntimeOverride(game.GameID); name != "" {
			return steamToolLabel(name)
		}
		switch installedPlatform(game) {
		case "linux":
			return "Linux nativo"
		case "windows":
			if name := steamRuntimeOverride("0"); name != "" {
				return steamToolLabel(name) + " (preferência global)"
			}
			return "Windows · Proton escolhido pela Steam ao iniciar"
		}
		return "Steam · modo ainda não identificado"
	case "gog", "epic":
		if install := heroic.InstallFor(game.GameID); install != nil {
			if strings.EqualFold(install.Platform, "linux") {
				return "Linux nativo"
			}
			if name := strings.TrimSpace(install.WineName); name != "" {
				return name
			}
			if name := runnerFromPath(install.WineBinary); name != "" {
				return name
			}
			if strings.EqualFold(install.Platform, "windows") {
				return "Windows · runner gerenciado pelo cliente"
			}
		}
		switch installedPlatform(game) {
		case "linux":
			return "Linux nativo"
		case "windows":
			return "Windows · runner gerenciado pelo cliente"
		}
	case "linux":
		if name := runnerFromPath(game.Executable); name != "" {
			return name
		}
		return "Linux nativo"
	case "emulation":
		return "Emulador"
	}
	return ""
}

func installedPlatform(game *models.Game) string {
	if !game.IsInstalled || game.InstallDirectory == "" {
		return ""
	}
	var linux, windows bool
	_ = filepath.WalkDir(game.InstallDirectory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			relative, _ := filepath.Rel(game.InstallDirectory, path)
			if relative != "." && strings.Count(relative, string(filepath.Separator)) >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		for _, helper := range []string{"crash", "setup", "unins", "redist", "vcredist", "steamerror"} {
			if strings.Contains(name, helper) {
				return nil
			}
		}
		switch {
		case strings.HasSuffix(name, ".exe"):
			windows = windows || hasSignature(path, "MZ")
		case !strings.Contains(name, ".so") && !strings.HasSuffix(name, ".dll"):
			if info, err := entry.Info(); err == nil && info.Mode().Perm()&0111 != 0 {
				linux = linux || hasSignature(path, "\x7fELF")
			}
		}
		if linux && windows {
			return filepath.SkipAll
		}
		return nil
	})
	if linux && !windows {
		return "linux"
	}
	if windows && !linux {
		return "windows"
	}
	return ""
}

func hasSignature(path, signature string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	header := make([]byte, len(signature))
	_, err = io.ReadFull(file, header)
	return err == nil && string(header) == signature
}

var steamToolVersion = regexp.MustCompile(`^proton_(\d+)(?:[._](\d+))?$`)

func steamToolLabel(name string) string {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "steamlinuxruntime") {
		return "Linux nativo · Steam Linux Runtime"
	}
	switch lower {
	case "proton_experimental", "proton - experimental":
		return "Proton Experimental"
	case "proton_hotfix", "proton - hotfix":
		return "Proton Hotfix"
	}
	if parts := steamToolVersion.FindStringSubmatch(lower); parts != nil {
		minor := parts[2]
		if minor == "" {
			minor = "0"
		}
		return "Proton " + parts[1] + "." + minor
	}
	return strings.TrimSuffix(strings.TrimSuffix(name, "-x86_64"), "-arm64")
}

func lastSteamProton(game *models.Game) string {
	if _, err := strconv.ParseUint(game.GameID, 10, 32); err != nil {
		return ""
	}
	for _, library := range steam.LibraryDirectories() {
		path := filepath.Join(library, "compatdata", game.GameID, "config_info")
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<10 {
			continue
		}
		if data, err := os.ReadFile(path); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				marker := strings.Index(line, "/files/")
				if marker > 0 {
					if name := runnerFromPath(line[:marker]); name != "" {
						return steamToolLabel(name)
					}
				}
			}
		}
	}
	return ""
}

func processRunner(process gameproc.Process) string {
	for _, path := range filepath.SplitList(process.Env["STEAM_COMPAT_TOOL_PATHS"]) {
		if name := runnerFromPath(path); strings.Contains(strings.ToLower(name), "proton") {
			return name
		}
	}
	for _, argument := range process.Cmdline {
		switch strings.ToLower(filepath.Base(argument)) {
		case "proton", "wine", "wine64", "wine-preloader", "wine64-preloader", "wineserver":
			name := runnerFromPath(argument)
			if name == "Wine" && process.Env["STEAM_COMPAT_DATA_PATH"] != "" {
				continue
			}
			return name
		}
	}
	if process.Env["STEAM_COMPAT_DATA_PATH"] != "" {
		return "Proton · versão não identificada"
	}
	return ""
}

func runnerFromPath(path string) string {
	if path == "" || strings.HasPrefix(path, "-") {
		return ""
	}
	leaf := filepath.Base(path)
	switch strings.ToLower(leaf) {
	case "wine", "wine64", "wine-preloader", "wine64-preloader", "wineserver":
		for dir := filepath.Dir(path); dir != "." && dir != "/"; dir = filepath.Dir(dir) {
			name := filepath.Base(dir)
			lower := strings.ToLower(name)
			if strings.Contains(lower, "proton") || strings.HasPrefix(lower, "wine-") {
				return name
			}
		}
		return "Wine"
	case "proton":
		name := filepath.Base(filepath.Dir(path))
		if strings.Contains(strings.ToLower(name), "proton") {
			return name
		}
		return "Proton · versão não identificada"
	}
	lower := strings.ToLower(leaf)
	if strings.HasPrefix(lower, "proton ") || strings.HasPrefix(lower, "proton-") ||
		strings.HasPrefix(lower, "ge-proton") || strings.HasPrefix(lower, "wine-") {
		return leaf
	}
	return ""
}

func steamRuntimeOverride(appID string) string {
	if _, err := strconv.ParseUint(appID, 10, 32); err != nil {
		return ""
	}
	for _, root := range steam.Roots() {
		path := filepath.Join(root, "config", "config.vdf")
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
			continue
		}
		if data, err := os.ReadFile(path); err == nil {
			if name := findSteamRuntime(string(data), appID); name != "" {
				return name
			}
		}
	}
	return ""
}

var steamVDFToken = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|[{}]|//[^\n]*`)

func findSteamRuntime(config, appID string) string {
	tokens := steamVDFToken.FindAllString(config, -1)
	for index, token := range tokens {
		if !strings.EqualFold(token, `"CompatToolMapping"`) || index+1 >= len(tokens) || tokens[index+1] != "{" {
			continue
		}
		depth := 1
		target := false
		for i := index + 2; i < len(tokens) && depth > 0; i++ {
			part := tokens[i]
			switch part {
			case "{":
				depth++
			case "}":
				depth--
				if depth == 1 {
					target = false
				}
			default:
				if depth == 1 && part == strconv.Quote(appID) && i+1 < len(tokens) && tokens[i+1] == "{" {
					target = true
				}
				if target && depth == 2 && strings.EqualFold(part, `"name"`) && i+1 < len(tokens) {
					value, err := strconv.Unquote(tokens[i+1])
					if err == nil {
						return value
					}
				}
			}
		}
	}
	return ""
}
