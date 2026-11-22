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
  supported: "Suportado",
  running: "Em execução",
  broken: "Incompatível",
  denied: "Banimento / Bloqueado",
  planned: "Planejado",
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
    tipText.textContent = "Atenção: Incompatível ou sujeito a banimento ao rodar via Proton/Wine no Linux.";
  } else if ((info.status || "").toLowerCase() === "running") {
    tipText.textContent = "Funcional no Linux, mas pode exigir ajustes ou quebrar em novos patches.";
  } else {
    tipText.textContent = "Totalmente suportado no Linux via Proton/Wine.";
  }
  tooltip.append(tipHead, tipText);
  banner.append(badge, tooltip);
  return banner;
}
