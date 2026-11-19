package app

import (
	"context"

	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/launcher"
	"github.com/playnux/core/runner/wine"
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

func (c *App) configuredRunner(ctx context.Context, game *models.Game) string {
	options, err := c.LaunchOptions(ctx, game)
	if err != nil {
		return ""
	}
	exe := wine.WindowsExecutable(game)
	switch options.Runner {
	case "proton":
		if name := wine.RunnerName(options.ProtonPath); name != "" {
			return name
		}
		return "Proton (indisponível)"
	case "wine":
		if exe != "" {
			return "Wine"
		}
	case "auto":
		if game.PluginID == "linux" && exe != "" {
			for _, candidate := range wine.DiscoverRunners() {
				if candidate.Kind == "proton" {
					return candidate.Name + " (automático)"
				}
			}
			return "Wine (automático)"
		}
	}
	return configuredRuntime(game)
}

