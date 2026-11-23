import { t } from "../../i18n";
export interface AntiCheatInfo {
  found: boolean;
  name?: string;
  status?: string;
  risk?: string;
  warning: boolean;
  anticheats?: string[];
  notes?: string[];
  url?: string;
  error?: string;
}

const STATUS_LABELS: Record<string, string> = {
  supported: t("Supported"),
  running: t("Running"),
  broken: t("Incompatible"),
  denied: t("Ban / Blocked"),
  planned: t("Planned"),
};

/** Builds an anti-cheat status/warning banner. Returns null when not relevant. */
export function renderAntiCheatBanner(info: AntiCheatInfo | null | undefined): HTMLElement | null {
  // Se o jogo não usa anti-cheat ou não está catalogado, não mostra badge para não poluir
  if (!info || !info.found || info.error || !(info.anticheats?.length)) {
    return null;
  }

  const risk = (info.risk || (info.warning ? "warning" : "safe")).toLowerCase();

  const banner = document.createElement("div");
  banner.className = `anticheat-chip risk-${risk}`;
  banner.tabIndex = 0;

  const badge = document.createElement("span");
  badge.className = `anticheat-badge badge-${risk}`;

  const cheats = info.anticheats.join(" + ");
  const status = STATUS_LABELS[(info.status || "").toLowerCase()] ?? info.status ?? "";
  badge.textContent = `AC: ${status}`;

  const tooltip = document.createElement("div");
  tooltip.className = "compat-tooltip";
  const tipHead = document.createElement("strong");
  tipHead.textContent = `${cheats} (${status})`;
  const tipText = document.createElement("p");
  if (info.warning) {
    tipText.textContent = t("Warning: incompatible or subject to bans when run through Proton or Wine on Linux.");
  } else if ((info.status || "").toLowerCase() === "running") {
    tipText.textContent = t("Works on Linux, with possible workarounds or regressions after updates.");
  } else {
    tipText.textContent = t("Fully supported on Linux through Proton or Wine.");
  }
  tooltip.append(tipHead, tipText);
  banner.append(badge, tooltip);
  return banner;
}
