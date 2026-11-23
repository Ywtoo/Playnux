import * as Host from "../bindings/github.com/playnux/ui/host/host.js";
import type { Game } from "../bindings/github.com/playnux/core/models/models.js";
import { renderAntiCheatBanner, type AntiCheatInfo } from "./plugins/anticheat";
import { renderProtonCard, type ProtonIntelligence } from "./plugins/protondb";

interface CompatibilityPayload {
  protondb?: ProtonIntelligence | null;
  anticheat?: AntiCheatInfo | null;
}

const PC_PLUGINS = new Set(["steam", "gog", "epic", "linux"]);
const cache = new Map<string, CompatibilityPayload>();
const pending = new Map<string, Promise<CompatibilityPayload>>();

function compatibleGame(game: Game): boolean {
  return PC_PLUGINS.has(game.plugin_id);
}

async function fetchCompatibility(game: Game): Promise<CompatibilityPayload> {
  const cached = cache.get(game.id);
  if (cached) return cached;
  const running = pending.get(game.id);
  if (running) return running;

  const request = Host.GetGameCompatibility(game.id)
    .then((payload) => {
      const value = (payload ?? {}) as CompatibilityPayload;
      cache.set(game.id, value);
      return value;
    })
    .finally(() => pending.delete(game.id));
  pending.set(game.id, request);
  return request;
}

/** Loads and renders ProtonDB and anti-cheat information for the selected game. */
export function renderCompatibility(container: HTMLElement, game: Game | null): void {
  const id = game?.id ?? "";
  if (container.dataset.gameId !== id) {
    container.dataset.gameId = id;
    container.replaceChildren();
    container.hidden = true;
  }
  if (!game || !compatibleGame(game)) return;

  void Promise.all([
    fetchCompatibility(game).catch(() => ({} as CompatibilityPayload)),
    game.is_installed ? Host.GetGameRuntime(game.id).catch(() => null) : null,
  ]).then(([payload, runtime]) => {
    if (container.dataset.gameId !== game.id) return;
    const antiCheat = renderAntiCheatBanner(payload.anticheat);
    const proton = renderProtonCard(payload.protondb, runtime?.running || runtime?.configured);
    const nodes = [antiCheat, proton].filter((node): node is HTMLElement => node !== null);
    container.replaceChildren(...nodes);
    container.hidden = !nodes.length;
  });
}
