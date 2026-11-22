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
  if (!intel || !intel.available || intel.error) {
    return null;
  }
  const tier = (intel.tier || "pending").toLowerCase();
  const card = document.createElement("section");
  card.className = `proton-card tier-${tier}`;
  card.setAttribute("aria-label", "Compatibilidade no ProtonDB");

  const header = document.createElement("div");
  header.className = "proton-card-head";
  const brand = document.createElement("span");
  brand.className = "proton-brand";
  brand.textContent = "ProtonDB";
  const badge = document.createElement("span");
  badge.className = `proton-tier tier-${tier}`;
  badge.textContent = TIER_LABELS[tier] ?? tier;
  const total = document.createElement("span");
  total.className = "proton-total";
  total.textContent = `${intel.total_reports} ${intel.total_reports === 1 ? "relato" : "relatos"}`;
  header.append(brand, badge, total);
  card.append(header);

  if (intel.recommended) {
    const recommendation = document.createElement("p");
    recommendation.className = "proton-recommendation";
    const label = document.createElement("span");
    label.textContent = "Recomendado: ";
    const version = document.createElement("strong");
    version.textContent = intel.recommended;
    recommendation.append(label, version);
    card.append(recommendation);

    if (intel.basis) {
      const basis = document.createElement("p");
      basis.className = "proton-basis";
      basis.textContent = intel.basis;
      card.append(basis);
    }

    const runner = (options.runners ?? []).find((item) => item.kind === "proton" && runnerMatches(intel.recommended ?? "", item.name));
    // Só exibe o botão de aplicar se o jogo estiver instalado, o runner estiver disponível E
    // o runner configurado atualmente for DIFERENTE do recomendado.
    const isAlreadyApplied = Boolean(options.configuredRunner && runner && runnerMatches(options.configuredRunner, runner.name));
    if (runner && options.onApply && options.canApply !== false && !isAlreadyApplied) {
      const apply = document.createElement("button");
      apply.type = "button";
      apply.className = "primary-button compact proton-apply";
      apply.textContent = `Aplicar ${runner.name} com 1 clique`;
      apply.addEventListener("click", () => options.onApply?.(runner.path));
      card.append(apply);
    }
  } else if (intel.basis) {
    const basis = document.createElement("p");
    basis.className = "proton-basis";
    basis.textContent = intel.basis;
    card.append(basis);
  }

  const recent = (intel.recent ?? []).filter((report) => Boolean(report.rating && report.proton_version)).slice(0, 4);
  if (recent.length) {
    const chips = document.createElement("div");
    chips.className = "proton-chips";
    for (const report of recent) {
      const chip = document.createElement("span");
      const tierKey = (report.rating || "").toLowerCase();
      chip.className = `proton-chip tier-${tierKey}`;
      chip.title = report.timestamp
        ? new Date(report.timestamp * 1000).toLocaleDateString("pt-BR")
        : "";
      chip.textContent = `${TIER_LABELS[tierKey] ?? "•"} · ${report.proton_version}`;
      chips.append(chip);
    }
    card.append(chips);
  }

  const link = document.createElement("button");
  link.type = "button";
  link.className = "link-button";
  link.textContent = "Abrir ProtonDB…";
  link.addEventListener("click", () => {
    void Browser.OpenURL(intel.url).catch(() => undefined);
  });
  card.append(link);
  return card;
}
