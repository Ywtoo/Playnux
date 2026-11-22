import { Browser } from "@wailsio/runtime";
import type { Game } from "../../../bindings/github.com/playnux/core/models/models.js";

export interface ProtonRecentReport {
  rating: string;
  proton_version: string;
  timestamp: number;
}

export interface ProtonIntelligence {
  app_id: string;
  available: boolean;
  tier: string;
  confidence?: string;
  total_reports: number;
  recommended?: string;
  recommended_tier?: string;
  basis?: string;
  recent?: ProtonRecentReport[];
  url: string;
  error?: string;
}

/** ProtonDB reports only exist for numeric Steam AppIDs. */
export function hasProtonReports(game: Game): boolean {
  return game.plugin_id === "steam" && /^[1-9]\d*$/.test(game.game_id);
}

export async function openProtonReports(game: Game): Promise<void> {
  if (hasProtonReports(game)) {
    await Browser.OpenURL(`https://www.protondb.com/app/${game.game_id}`);
  }
}

const TIER_LABELS: Record<string, string> = {
  platinum: "Platinum",
  gold: "Gold",
  silver: "Silver",
  bronze: "Bronze",
  borked: "Borked",
  pending: "Pendente",
};

function normalized(value: string): string {
  return value.toLocaleLowerCase("pt-BR").replace(/[^a-z0-9]/g, "");
}

export function runnerMatches(recommended: string, runnerName: string): boolean {
  const left = normalized(recommended);
  const right = normalized(runnerName);
  return left !== "" && (left === right || left.includes(right) || right.includes(left));
}

interface BannerOptions {
  onApply?: (runnerPath: string) => void;
  runners?: { name: string; path: string; kind: string }[];
  canApply?: boolean;
  configuredRunner?: string;
}

/** Builds the ProtonDB hero card. Returns null when there is nothing useful. */
export function renderProtonCard(intel: ProtonIntelligence | null | undefined, options: BannerOptions = {}): HTMLElement | null {
  const container = document.createElement("div");
  container.className = "proton-chip-badge";
  container.tabIndex = 0;

  const currentRunner = options.configuredRunner || "Padrão da loja";
  const recommended = intel?.recommended || "";
  const isDiff = Boolean(recommended && currentRunner && !runnerMatches(recommended, currentRunner));

  const badge = document.createElement("span");
  badge.className = `proton-version-tag ${isDiff ? "tag-warning" : "tag-normal"}`;
  badge.textContent = `Proton: ${currentRunner}`;

  const tooltip = document.createElement("div");
  tooltip.className = "compat-tooltip";
  const head = document.createElement("strong");
  head.textContent = intel?.available ? `ProtonDB: ${TIER_LABELS[intel.tier?.toLowerCase() ?? ""] ?? intel.tier}` : "Proton";
  const desc = document.createElement("p");
  if (recommended) {
    desc.textContent = isDiff
      ? `Recomendado no ProtonDB: ${recommended}. Versão em uso é diferente (Propriedades para alterar).`
      : `Compatibilidade ideal com ${recommended}.`;
  } else {
    desc.textContent = "Executando com as configurações atuais de compatibilidade.";
  }
  tooltip.append(head, desc);

  container.append(badge, tooltip);
  return container;
}
