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
  supported: "Supported",
  running: "Running",
  broken: "Broken",
  denied: "Denied",
  planned: "Planned",
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

  const icon = document.createElement("span");
  icon.className = "anticheat-icon";
  icon.setAttribute("aria-hidden", "true");
  icon.textContent = info.warning ? "⚠" : "🛡";

  const body = document.createElement("div");
  body.className = "anticheat-body";
  const title = document.createElement("strong");
  const cheats = info.anticheats.join(" + ");
  const status = STATUS_LABELS[(info.status || "").toLowerCase()] ?? info.status ?? "";
  title.textContent = info.warning ? "Aviso de anti-cheat" : "Anti-cheat";
  const text = document.createElement("p");
  if (info.warning) {
    text.textContent = `Este jogo usa ${cheats}. Rodar via Proton/Wine no Linux pode não conectar aos servidores ou causar suspensão da conta. Status atual: ${status}. Verifique antes de jogar online.`;
  } else if ((info.status || "").toLowerCase() === "running") {
    text.textContent = `${cheats} · ${status}. Funciona no Linux, mas pode exigir ajustes ou quebrar após atualizações.`;
  } else {
    text.textContent = `${cheats} · ${status} no Linux/Wine.`;
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
    link.textContent = "Ver status no AreWeAntiCheatYet…";
    link.addEventListener("click", () => {
      void Browser.OpenURL(info.url ?? "").catch(() => undefined);
    });
    body.append(link);
  }

  banner.append(icon, body);
  return banner;
}
