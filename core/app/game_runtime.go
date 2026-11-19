package app

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/gameproc"
	"github.com/playnux/core/store/builtins/heroic"
)

type GameRuntime struct {
	Configured string `json:"configured"`
	Running    string `json:"running"`
}

func (c *App) GameRuntime(ctx context.Context, id string) (GameRuntime, error) {
	game, err := c.GetGame(ctx, id)
	if err != nil {
		return GameRuntime{}, err
	}
	result := GameRuntime{Configured: c.configuredRunner(ctx, game)}
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
			return name
		}
		return "Padrão da Steam"
	case "gog", "epic":
		if install := heroic.InstallFor(game.GameID); install != nil {
			if name := strings.TrimSpace(install.WineName); name != "" {
				return name
			}
			if name := runnerFromPath(install.WineBinary); name != "" {
				return name
			}
		}
	case "linux":
		return runnerFromPath(game.Executable)
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
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, root := range []string{".local/share/Steam", ".steam/steam", ".var/app/com.valvesoftware.Steam/.local/share/Steam"} {
		path := filepath.Join(home, root, "config", "config.vdf")
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
