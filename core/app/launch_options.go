package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/playnux/core/database"
	"github.com/playnux/core/models"
	"github.com/playnux/core/runner"
	"github.com/playnux/core/runner/launcher"
)

const launchOptionsPrefix = "launch.options."

func (c *App) LaunchOptions(ctx context.Context, game *models.Game) (launcher.Options, error) {
	var options launcher.Options
	if !runner.AllowLocal(game) {
		return options, nil
	}
	raw, err := c.db.GetSetting(ctx, launchOptionsPrefix+game.ID)
	if errors.Is(err, database.ErrSettingNotFound) {
		return options, nil
	}
	if err != nil {
		return options, err
	}
	err = json.Unmarshal([]byte(raw), &options)
	return options, err
}

func (c *App) SaveLaunchOptions(ctx context.Context, id, directory string, arguments []string, runnerName, protonPath string) error {
	game, err := c.GetGame(ctx, id)
	if err != nil {
		return err
	}
	if !runner.AllowLocal(game) {
		return errors.New("a configuração de inicialização pertence ao cliente da loja")
	}
	options, err := runner.Validate(game, directory, arguments, runnerName, protonPath)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return err
	}
	return c.db.SetSetting(ctx, launchOptionsPrefix+game.ID, string(raw))
}
