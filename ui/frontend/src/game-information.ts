import type { Game } from "../bindings/github.com/playnux/core/models/models.js";
import { formatPlaytime, formatSize, lastPlayedLabel, plainText, releaseYear, sourceLabel } from "./format";

function releaseLabel(game: Game): string {
  if (game.release_date) {
    const date = new Date(game.release_date);
    if (!Number.isNaN(date.getTime())) {
      return new Intl.DateTimeFormat("pt-BR", { dateStyle: "long", timeZone: "UTC" }).format(date);
    }
  }
  return releaseYear(game);
}

function names(values: string[] | null | undefined): string {
  return [...new Set(values?.map((value) => value.trim()).filter(Boolean))].join(", ");
}

function score(value: number | null | undefined): string {
  return typeof value === "number" && Number.isFinite(value) ? `${value}/100` : "";
}

export function renderGameInformation(game: Game, description: HTMLElement, sheet: HTMLElement, tags: HTMLElement): void {
  description.textContent = plainText(game.description) || "Sinopse indisponível.";
  const tagNames = [...new Set(game.tags?.map((value) => value.trim()).filter(Boolean))];
  tags.hidden = !tagNames.length;
  tags.replaceChildren();
  if (tagNames.length) {
    const heading = document.createElement("h3");
    heading.textContent = "Tags";
    const list = document.createElement("ul");
    for (const name of tagNames) {
      const item = document.createElement("li");
      item.textContent = name;
      list.append(item);
    }
    tags.append(heading, list);
  }
  const groups: { title: string; entries: [string, string][] }[] = [
    { title: "Ficha técnica", entries: [
      ["Lançamento", releaseLabel(game)],
      ["Desenvolvedoras", names(game.developers)],
      ["Distribuidoras", names(game.publishers)],
      ["Gêneros", names(game.genres)],
      ["Plataformas", names(game.platforms)],
      ["Recursos", names(game.features)],
      ["Categorias", names(game.categories)],
      ["Versão", game.version],
    ] },
    { title: "Avaliações", entries: [
      ["Crítica", score(game.critic_score)],
      ["Comunidade", score(game.community_score)],
      ["Sua nota", score(game.user_score)],
    ] },
    { title: "Na sua biblioteca", entries: [
      ["Origem", sourceLabel(game)],
      ["Instalação", game.is_installed ? "Instalado" : "Não instalado"],
      ["Tamanho", formatSize(game.install_size)],
      ["Tempo de jogo", formatPlaytime(game.playtime)],
      ["Última sessão", lastPlayedLabel(game.last_activity)],
      ["Vezes iniciado", game.play_count > 0 ? String(game.play_count) : ""],
    ] },
  ];

  const sections = groups.flatMap(({ title, entries }) => {
    const populated = entries.filter(([, value]) => value);
    if (!populated.length) return [];
    const group = document.createElement("section");
    group.className = "game-data-group";
    const heading = document.createElement("h3");
    heading.textContent = title;
    const list = document.createElement("dl");
    for (const [label, value] of populated) {
      const term = document.createElement("dt");
      term.textContent = label;
      const definition = document.createElement("dd");
      definition.textContent = value;
      list.append(term, definition);
    }
    group.append(heading, list);
    return [group];
  });
  sheet.replaceChildren(...sections);
}

export type ArtworkField = "cover" | "background" | "logo" | "icon";
