package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/playnux/core/metadata/anticheat"
	"github.com/playnux/core/metadata/protondb"
)

// GameCompatibility groups the online compatibility intelligence shown in the
// game details panel. Network failures are reported inside the payload so the
// game still opens normally.
type GameCompatibility struct {
	ProtonDB  *protondb.Intelligence `json:"protondb,omitempty"`
	AntiCheat *anticheat.Info        `json:"anticheat,omitempty"`
}

// GameCompatibility fetches ProtonDB reports for Steam games and anti-cheat
// status for PC games. Results are cached by the underlying clients.
func (c *App) GameCompatibility(ctx context.Context, id string) (GameCompatibility, error) {
	game, err := c.GetGame(ctx, id)
	if err != nil {
		return GameCompatibility{}, err
	}
	result := GameCompatibility{}

	if game.PluginID == "steam" {
		if _, err := strconv.ParseUint(game.GameID, 10, 64); err == nil {
			intel, fetchErr := c.protonDB.Fetch(ctx, game.GameID)
			if fetchErr != nil {
				intel = protondb.Intelligence{
					AppID: game.GameID,
					URL:   "https://www.protondb.com/app/" + game.GameID,
					Error: fetchErr.Error(),
				}
			}
			for _, platform := range game.Platforms {
				if strings.EqualFold(strings.TrimSpace(platform), "linux") {
					intel.NativeAvailable = true
				}
			}
			result.ProtonDB = &intel
		}
	}

	switch game.PluginID {
	case "steam", "gog", "epic", "linux":
		info, lookupErr := c.antiCheat.Lookup(ctx, game.Name, game.PluginID, game.GameID)
		if lookupErr != nil {
			info = anticheat.Info{Error: lookupErr.Error()}
		}
		result.AntiCheat = &info
	}

	return result, nil
}
