package app

import (
	"context"
	"os"

	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/launcher"
	"github.com/playnux/core/runner/wine"
	"github.com/playnux/core/store/builtins/steam"
)

// CompatibilityRunners lista Proton (Steam/GE) e o Wine do sistema. A UI usa
// essa lista para deixar explícito qual motor iniciará o jogo.
func (c *App) CompatibilityRunners(context.Context) []wine.Runner {
	return wine.DiscoverRunners()
}

// applyCompatibility turns the saved launch options into an isolated Proton or
// Wine invocation when the user picked a custom runner.
func (c *App) applyCompatibility(game *models.Game, options *launcher.Options) error {
	return wine.Apply(options, game, c.dataDir)
}

// WindowsExecutable returns the .exe resolved for this game, if any. The UI
// uses it to decide whether a custom Proton runner can be chosen.
func (c *App) WindowsExecutable(ctx context.Context, id string) (string, error) {
	game, err := c.GetGame(ctx, id)
	if err != nil {
		return "", err
	}
	return wine.WindowsExecutable(game), nil
}

func (c *App) configuredRunner(ctx context.Context, game *models.Game, automatic string) string {
	options, err := c.LaunchOptions(ctx, game)
	if err != nil {
		return automatic
	}
	switch options.Runner {
	case "proton":
		if name := wine.RunnerName(options.ProtonPath); name != "" {
			if info, err := os.Stat(options.ProtonPath); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
				return name
			}
			return name + " (indisponível)"
		}
		return "Proton indisponível"
	case "wine":
		return "Wine do sistema"
	}
	return automatic
}

func automaticRunner(game *models.Game) string {
	if game.PluginID == "linux" && wine.ManualWindowsExecutable(game) != "" {
		for _, candidate := range wine.DiscoverRunners() {
			if candidate.Kind == "proton" {
				return candidate.Name
			}
		}
		return "Wine do sistema"
	}
	return configuredRuntime(game)
}

func (c *App) TrailerSources(ctx context.Context, id string, highQuality bool) ([]string, error) {
	game, err := c.GetGame(ctx, id)
	if err != nil {
		return nil, err
	}
	if provider, ok := c.providers[game.PluginID].(*steam.SteamProvider); ok {
		return provider.TrailerSources(ctx, game.GameID, highQuality), nil
	}
	return nil, nil
}
