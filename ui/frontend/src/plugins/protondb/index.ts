import { t } from "../../i18n";
import { Browser } from "@wailsio/runtime";
import type { Game } from "../../../bindings/github.com/playnux/core/models/models.js";

import type { Intelligence } from "../../../bindings/github.com/playnux/core/metadata/protondb/models.js";

export type ProtonIntelligence = Intelligence;

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
  pending: t("Pending"),
  native: t("Native Linux"),
};

function normalized(value: string): string {
  return value.toLowerCase()
    .replace(/-(?:x86_64|arm64)\b/g, "")
    .replace(/\s*\([^)]*\)\s*$/, "")
    .replace(/[^a-z0-9.]/g, "");
}

export function runtimeLabel(value: string): string {
  if (value.startsWith("Linux nativo")) return value.replace("Linux nativo", t("Native Linux"));
  switch (value) {
    case "Windows · Proton escolhido pela Steam ao iniciar": return t("Windows · Steam selects Proton at launch");
    case "Steam · modo ainda não identificado": return t("Steam · launch mode undetected");
    case "Proton · versão não identificada": return t("Proton · version undetected");
    case "Wine do sistema": return t("System Wine");
    default: return value;
  }
}

export function runnerMatches(recommended: string, name: string): boolean {
  return Boolean(recommended && name && normalized(recommended) === normalized(name));
}

export function recommendationText(intel?: ProtonIntelligence | null, runtime = ""): string {
  const native = /^Linux nativo/i.test(runtime);
  const prefix = native ? t("Launches natively on Linux.")
    : intel?.native_available || intel?.tier === "native" ? t("A native Linux edition is available.") : "";
  if (intel?.recommended) {
    const sample = intel.positive_reports && intel.sample_size
      ? t("  ({0} positive out of {1} recent reports)", intel.positive_reports, intel.sample_size) : "";
    const label = prefix ? t("Alternative for the Windows edition") : t("Highest-rated recent Proton version");
    return [prefix, `${label}: ${intel.recommended}${sample}.`].filter(Boolean).join(" ");
  }
  if (prefix) return prefix;
  return intel?.available ? t("No specific version stands out in the reports.") : t("Reports unavailable.");
}

export const officialProtonDownloads = new Map([
  ["Proton 11.0", "4628710"],
  ["Proton 10.0", "3658110"],
  ["Proton 9.0", "2805730"],
  ["Proton 8.0", "2348590"],
  ["Proton 7.0", "1887720"],
  ["Proton Experimental", "1493710"],
  ["Proton Hotfix", "2180100"],
]);

export function protonDownload(version: string): { label: string; url: string } {
  const family = /^proton[ -]+(\d+\.\d+)/i.exec(version);
  const appID = officialProtonDownloads.get(family ? `Proton ${family[1]}` : version);
  return appID
    ? { label: t("Download on Steam…"), url: `steam://install/${appID}` }
    : { label: t("Get ProtonUp-Qt…"), url: "https://flathub.org/apps/net.davidotek.pupgui2" };
}

export function renderProtonCard(
  intel: ProtonIntelligence | null | undefined,
  runtime = "",
): HTMLElement | null {
  if (!intel && !runtime) return null;
  const container = document.createElement("div");
  container.className = "proton-chip-badge";
  container.tabIndex = 0;

  const badge = document.createElement("span");
  const native = /^Linux nativo/i.test(runtime);
  const nativeAvailable = intel?.native_available || intel?.tier === "native";
  const knownProton = /^(?:GE-)?Proton[ -]/i.test(runtime) && !/indisponível|não identificada/i.test(runtime);
  const comparison = intel?.recommended ?? "";
  const branch = /^proton[ -]+(\d+\.\d+)(?:\s*\([^)]*\))?$/i.exec(runtime);
  const reportBranch = /^proton[ -]+(\d+\.\d+)-/i.exec(comparison);
  const sameBranch = Boolean(branch && reportBranch && branch[1] === reportBranch[1]);
  const mismatch = knownProton && Boolean(comparison) && !sameBranch && !runnerMatches(comparison, runtime);
  badge.className = `proton-version-tag ${native ? "tag-native" : mismatch ? "tag-warning" : "tag-normal"}`;
  const label = document.createElement("span");
  label.className = "proton-label";
  label.textContent = runtime ? t("Runtime") : "ProtonDB";
  const value = document.createElement("strong");
  value.textContent = native ? t("Native Linux") : runtimeLabel(runtime) ||
    (nativeAvailable ? t("Native Linux available") : intel?.available ? TIER_LABELS[intel.tier] ?? intel.tier : t("No reports"));
  badge.append(label, value);
  if (mismatch) {
    const flag = document.createElement("span");
    flag.className = "proton-report-flag";
    flag.textContent = t("Differs from reports");
    badge.append(flag);
  }

  const tooltip = document.createElement("div");
  tooltip.className = "compat-tooltip";
  const title = document.createElement("strong");
  title.textContent = intel?.available ? `ProtonDB · ${TIER_LABELS[intel.tier] ?? intel.tier}` : t("Compatibility");
  const description = document.createElement("p");
  description.textContent = intel ? recommendationText(intel, runtime) : runtimeLabel(runtime);
  tooltip.append(title, description);
  container.append(badge, tooltip);
  return container;
}
