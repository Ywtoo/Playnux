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

async function applyRecommended(game: Game, runnerPath: string, notify: (message: string, error?: boolean) => void, runnerName: string): Promise<void> {
  try {
    await Host.SaveLaunchOptions(game.id, "", [], "proton", runnerPath);
    notify(`${runnerName} aplicado em ${game.name}.`);
  } catch (error) {
    notify(error instanceof Error ? error.message : String(error), true);
  }
}

function paint(container: HTMLElement, game: Game, payload: CompatibilityPayload, notify: (message: string, error?: boolean) => void, runners: { name: string; path: string; kind: string }[], configuredRunner = ""): void {
  if (container.dataset.gameId !== game.id) return;
  const nodes: HTMLElement[] = [];
  const antiCheat = renderAntiCheatBanner(payload.anticheat);
  if (antiCheat) nodes.push(antiCheat);

  const proton = renderProtonCard(payload.protondb, {
    runners,
    canApply: game.is_installed,
    configuredRunner,
    onApply: (runnerPath) => {
      const runner = runners.find((item) => item.path === runnerPath);
      void applyRecommended(game, runnerPath, notify, runner?.name ?? "Proton");
    },
  });
  if (proton) nodes.push(proton);

  container.replaceChildren(...nodes);
  container.hidden = nodes.length === 0;
}

/** Loads and renders ProtonDB/anti-cheat banners for the details and drawer. */
export function renderCompatibility(container: HTMLElement, game: Game | null, notify: (message: string, error?: boolean) => void): void {
  const id = game?.id ?? "";
  if (container.dataset.gameId !== id) {
    container.dataset.gameId = id;
    container.replaceChildren();
    container.hidden = true;
  }
  if (!game || !compatibleGame(game)) {
    return;
  }

  const cached = cache.get(game.id);
  const render = (payload: CompatibilityPayload, runners: { name: string; path: string; kind: string }[], configured = "") => {
    paint(container, game, payload, notify, runners, configured);
  };

  const getRunnerState = async () => {
    try {
      const [runners, runtime] = await Promise.all([
        Host.GetCompatibilityRunners().catch(() => []),
        Host.GetGameRuntime(game.id).catch(() => null),
      ]);
      return {
        runners: (runners ?? []) as { name: string; path: string; kind: string }[],
        configured: runtime?.configured || runtime?.running || "",
      };
    } catch {
      return { runners: [], configured: "" };
    }
  };

  if (cached) {
    if (cached.protondb?.recommended) {
      void getRunnerState().then(({ runners, configured }) => render(cached, runners, configured));
    } else {
      render(cached, []);
    }
    return;
  }

  void fetchCompatibility(game).then(async (payload) => {
    const { runners, configured } = payload.protondb?.recommended
      ? await getRunnerState()
      : { runners: [], configured: "" };
    render(payload, runners, configured);
  }).catch(() => undefined);
}
