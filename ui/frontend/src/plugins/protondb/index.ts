import { Browser } from "@wailsio/runtime";
import type { Game } from "../../../bindings/github.com/playnux/core/models/models.js";

export function hasProtonReports(game: Game): boolean {
  return game.plugin_id === "steam" && /^[1-9]\d*$/.test(game.game_id);
}

export async function openProtonReports(game: Game): Promise<void> {
  if (hasProtonReports(game)) {
    await Browser.OpenURL(`https://www.protondb.com/app/${game.game_id}`);
  }
}
