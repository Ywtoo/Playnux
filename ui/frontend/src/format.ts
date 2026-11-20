import type { Game } from "../bindings/github.com/playnux/core/models/models";

const SOURCE_LABELS: Record<string, string> = {
  steam: "Steam",
  epic: "Epic Games",
  gog: "GOG",
  xbox: "Xbox",
  linux: "Linux",
  local: "DRM-free",
  flatpak: "Flatpak",
  manual: "Manual",
  emulation: "Emulação",
  "emulação": "Emulação",
};

const SOURCE_CLASSES: Record<string, string> = {
  steam: "steam",
  epic: "epic",
  gog: "gog",
  xbox: "xbox",
  linux: "linux",
  local: "local",
  flatpak: "flatpak",
  manual: "manual",
  emulation: "emulation",
  "emulação": "emulation",
};

function keyOf(game: Game): string {
  const source = (game.source || "").trim().toLowerCase();
  if (SOURCE_LABELS[source]) {
    return source;
  }
  return (game.plugin_id || game.source || "").trim().toLowerCase();
}

export function sourceLabel(game: Game): string {
  const key = keyOf(game);
  if (SOURCE_LABELS[key]) {
    return SOURCE_LABELS[key];
  }
  return game.source || game.plugin_id || "Biblioteca";
}

export function sourceClass(game: Game): string {
  const key = keyOf(game);
  if (SOURCE_CLASSES[key]) {
    return SOURCE_CLASSES[key];
  }
  return key.replace(/[^a-z0-9]+/g, "-");
}

export function initials(name: string): string {
  const words = name
    .replace(/[^\p{L}\p{N} ]+/gu, " ")
    .split(/\s+/)
    .filter(Boolean);
  if (words.length === 0) {
    return "?";
  }
  if (words.length === 1) {
    return words[0].slice(0, 2).toUpperCase();
  }
  return (words[0][0] + words[words.length - 1][0]).toUpperCase();
}

export function formatPlaytime(seconds: number): string {
  if (!seconds || seconds < 60) {
    return "";
  }
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  if (hours === 0) {
    return `${minutes} min`;
  }
  const rest = minutes % 60;
  return rest === 0 ? `${hours} h` : `${hours} h ${rest} min`;
}

export function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) {
    return "";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  const digits = value >= 10 || unit === 0 ? 0 : 1;
  return `${value.toFixed(digits).replace(".", ",")} ${units[unit]}`;
}

export function releaseYear(game: Game): string {
  if (game.release_year > 0) {
    return String(game.release_year);
  }
  if (game.release_date) {
    const match = /^(\d{4})/.exec(game.release_date);
    if (match) {
      return match[1];
    }
  }
  return "";
}

export function lastPlayedLabel(value: string | null | undefined): string {
  if (!value) {
    return "Nunca jogado";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const today = new Date();
  const start = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const target = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const days = Math.round((start.getTime() - target.getTime()) / 86_400_000);
  if (days === 0) {
    return "Jogado hoje";
  }
  if (days === 1) {
    return "Jogado ontem";
  }
  if (days > 1 && days < 30) {
    return `Jogado há ${days} dias`;
  }
  return `Jogado em ${date.toLocaleDateString("pt-BR", { day: "2-digit", month: "short", year: "numeric" })}`;
}

export function cardSubtitle(game: Game): string {
  const playtime = formatPlaytime(game.playtime);
  return playtime ? `${sourceLabel(game)} · ${playtime}` : sourceLabel(game);
}

// Descrições das lojas podem vir em HTML. A UI mostra texto puro (sem
// interpretar marcação vinda da rede).
export function plainText(value: string): string {
  if (!value) {
    return "";
  }
  const withBreaks = value
    .replace(/<br\s*\/?>/gi, "\n")
    .replace(/<\/(p|div|li|h[1-6])>/gi, "\n");
  const parsed = new DOMParser().parseFromString(withBreaks, "text/html");
  const text = parsed.body ? parsed.body.textContent || "" : "";
  return text
    .replace(/[ \t]+/g, " ")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

export function canLaunch(game: Game): boolean {
  const key = keyOf(game);
  if (key === "xbox") {
    return false;
  }
  if ((key === "emulation" || key === "emulação" || game.plugin_id === "emulation") && !game.executable) {
    return false;
  }
  return true;
}

export function launchBlockReason(game: Game): string {
  const key = keyOf(game);
  if (key === "xbox") {
    return "Jogos Xbox não executam nativamente no Linux";
  }
  if ((key === "emulation" || key === "emulação" || game.plugin_id === "emulation") && !game.executable) {
    return "Nenhum emulador compatível foi encontrado para esta ROM";
  }
  return "";
}
