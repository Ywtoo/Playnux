import { Browser } from "@wailsio/runtime";

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
  if (!info || !info.found || info.error || !(info.anticheats?.length)) {
    return null;
  }
  const risk = (info.risk || (info.warning ? "warning" : "safe")).toLowerCase();
  const banner = document.createElement("section");
  banner.className = `anticheat-banner risk-${risk}`;
  banner.setAttribute("role", info.warning ? "alert" : "status");

  const badge = document.createElement("span");
  badge.className = `anticheat-badge badge-${risk}`;
  const status = STATUS_LABELS[(info.status || "").toLowerCase()] ?? info.status ?? "";
  badge.textContent = status;

  const body = document.createElement("div");
  body.className = "anticheat-body";
  const title = document.createElement("div");
  title.className = "anticheat-head";
  const titleText = document.createElement("strong");
  const cheats = info.anticheats.join(" + ");
  titleText.textContent = `Anti-Cheat: ${cheats}`;
  title.append(titleText, badge);

  const text = document.createElement("p");
  if (info.warning) {
    text.textContent = `Rodar via Proton/Wine no Linux pode não conectar aos servidores ou causar banimento da conta.`;
  } else if ((info.status || "").toLowerCase() === "running") {
    text.textContent = `Funciona no Linux com Proton, mas pode exigir configurações extras ou quebrar com novos patches.`;
  } else {
    text.textContent = `Totalmente funcional no Linux via Proton/Wine.`;
  }
  body.append(title, text);

  if (info.notes?.length) {
    const note = document.createElement("p");
    note.className = "anticheat-note";
    note.textContent = info.notes[0];
    body.append(note);
  }

  if (info.url) {
    const link = document.createElement("button");
    link.type = "button";
    link.className = "link-button";
    link.textContent = "Ver no AreWeAntiCheatYet…";
    link.addEventListener("click", () => {
      void Browser.OpenURL(info.url ?? "").catch(() => undefined);
    });
    body.append(link);
  }

  banner.append(body);
  return banner;
}
