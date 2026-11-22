import { Window } from "@wailsio/runtime";
import type { Game } from "../bindings/github.com/playnux/core/models/models";
import {
  canLaunch,
  formatPlaytime,
  initials,
  launchBlockReason,
  lastPlayedLabel,
  releaseYear,
  sourceLabel,
} from "./format";
import { trailerEmbedURL } from "./media";

export interface BigPictureOptions {
  root: HTMLElement;
  onLaunch: (game: Game) => void;
  onExit?: () => void;
  onError: (error: unknown) => void;
}

export interface BigPictureController {
  open(games: Game[], selectedID?: string): Promise<void>;
  close(): void;
  isOpen(): boolean;
  setGames(games: Game[]): void;
}

interface PadState {
  left: boolean;
  right: boolean;
  up: boolean;
  down: boolean;
  a: boolean;
  b: boolean;
}

const EMPTY_PAD: PadState = {
  left: false,
  right: false,
  up: false,
  down: false,
  a: false,
  b: false,
};

function element<T extends HTMLElement>(root: ParentNode, selector: string): T {
  const found = root.querySelector<T>(selector);
  if (!found) {
    throw new Error(`Big Picture: elemento ${selector} não encontrado`);
  }
  return found;
}

export function createBigPicture(options: BigPictureOptions): BigPictureController {
  const { root, onLaunch, onExit, onError } = options;
  const background = element<HTMLImageElement>(root, "#big-bg");
  const logo = element<HTMLImageElement>(root, "#big-logo");
  const title = element<HTMLElement>(root, "#big-title");
  const meta = element<HTMLElement>(root, "#big-meta");
  const source = element<HTMLElement>(root, "#big-source");
  const play = element<HTMLButtonElement>(root, "#big-play");
  const video = element<HTMLElement>(root, "#big-video");
  const carousel = element<HTMLElement>(root, "#big-carousel");
  const closeButton = element<HTMLButtonElement>(root, "#big-close");
  const content = element<HTMLElement>(root, ".big-content");

  let active = false;
  let changingFullscreen = false;
  let restoreWindow = false;
  let games: Game[] = [];
  let index = 0;
  let padTimer = 0;
  let lastPad: PadState = { ...EMPTY_PAD };

  function selected(): Game | null {
    return games[index] || null;
  }

  async function renderVideo(game: Game): Promise<void> {
    video.replaceChildren();
    try {
      const settings = await Host.GetUISettings();
      if (!settings.big_picture_trailer) {
        video.hidden = true;
        return;
      }
    } catch {
      // continua se não conseguir ler
    }
    const url = trailerEmbedURL(game.trailer_url || "", {
      autoplay: true,
      muted: true,
      background: true,
    });
    if (!url) {
      video.hidden = true;
      return;
    }
    const frame = document.createElement("iframe");
    frame.src = url;
    frame.title = `Trailer de ${game.name}`;
    frame.allow = "autoplay; encrypted-media; picture-in-picture";
    frame.referrerPolicy = "strict-origin-when-cross-origin";
    video.append(frame);
    video.hidden = false;
  }

  function renderCarousel(): void {
    const items = games.map((game, itemIndex) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "big-item";
      button.dataset.index = String(itemIndex);
      button.setAttribute("aria-label", game.name);
      button.classList.toggle("is-selected", itemIndex === index);

      const cover = document.createElement("div");
      cover.className = "big-item-cover";
      if (game.cover_image) {
        const image = document.createElement("img");
        image.src = game.cover_image;
        image.alt = "";
        image.loading = "lazy";
        cover.append(image);
      } else {
        cover.textContent = initials(game.name);
      }
      const label = document.createElement("span");
      label.textContent = game.name;
      button.append(cover, label);
      return button;
    });
    carousel.replaceChildren(...items);
    const activeItem = carousel.querySelector<HTMLElement>(`.big-item[data-index="${index}"]`);
    activeItem?.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "center" });
  }

  function render(): void {
    const game = selected();
    if (!game) {
      title.textContent = "Sua biblioteca está vazia";
      meta.textContent = "";
      source.textContent = "";
      play.disabled = true;
      background.removeAttribute("src");
      logo.hidden = true;
      video.replaceChildren();
      video.hidden = true;
      carousel.replaceChildren();
      return;
    }

    document.title = `${game.name} — Playnux Fullscreen`;
    const backdrop = game.background_image || game.cover_image || "";
    if (backdrop) {
      background.src = backdrop;
    } else {
      background.removeAttribute("src");
    }
    background.alt = "";

    title.textContent = game.name;
    logo.onerror = () => {
      logo.hidden = true;
      title.hidden = false;
    };
    if (game.logo_image) {
      logo.src = game.logo_image;
      logo.hidden = false;
      title.hidden = true;
    } else {
      logo.removeAttribute("src");
      logo.hidden = true;
      title.hidden = false;
    }
    source.textContent = sourceLabel(game);

    const metaParts = [
      releaseYear(game),
      formatPlaytime(game.playtime),
      lastPlayedLabel(game.last_activity),
      ...(Array.isArray(game.genres) ? game.genres.slice(0, 2) : []),
    ].filter(Boolean);
    meta.textContent = metaParts.join("  ·  ");

    play.disabled = !canLaunch(game);
    play.title = canLaunch(game) ? `Jogar ${game.name}` : launchBlockReason(game);
    play.textContent = canLaunch(game) ? "▶  JOGAR" : "INDISPONÍVEL";

    renderVideo(game);
    renderCarousel();
    content.classList.remove("is-entering");
    void content.offsetWidth;
    content.classList.add("is-entering");
  }

  function move(delta: number): void {
    if (games.length === 0) {
      return;
    }
    index = (index + delta + games.length) % games.length;
    render();
  }

  function launch(): void {
    const game = selected();
    if (game) {
      onLaunch(game);
    }
  }

  function readPad(): PadState {
    if (typeof navigator.getGamepads !== "function") {
      return { ...EMPTY_PAD };
    }
    let pad: Gamepad | null = null;
    try {
      pad = Array.from(navigator.getGamepads()).find((candidate) => candidate !== null) || null;
    } catch {
      return { ...EMPTY_PAD };
    }
    if (!pad) {
      return { ...EMPTY_PAD };
    }
    const pressed = (button: number): boolean => Boolean(pad?.buttons[button]?.pressed);
    const axis = (value: number | undefined): number => value || 0;
    return {
      left: pressed(14) || axis(pad.axes[0]) < -0.55,
      right: pressed(15) || axis(pad.axes[0]) > 0.55,
      up: pressed(12) || axis(pad.axes[1]) < -0.55,
      down: pressed(13) || axis(pad.axes[1]) > 0.55,
      a: pressed(0),
      b: pressed(1),
    };
  }

  function pollGamepad(): void {
    if (!active) {
      return;
    }
    const current = readPad();
    if ((current.left || current.up) && !(lastPad.left || lastPad.up)) {
      move(-1);
    }
    if ((current.right || current.down) && !(lastPad.right || lastPad.down)) {
      move(1);
    }
    if (current.a && !lastPad.a) {
      launch();
    }
    if (current.b && !lastPad.b) {
      close();
      return;
    }
    lastPad = current;
  }

  function onKeyDown(event: KeyboardEvent): void {
    if (!active) {
      return;
    }
    switch (event.key) {
      case "ArrowLeft":
      case "ArrowUp":
        event.preventDefault();
        move(-1);
        break;
      case "ArrowRight":
      case "ArrowDown":
        event.preventDefault();
        move(1);
        break;
      case "Home":
        event.preventDefault();
        index = 0;
        render();
        break;
      case "End":
        event.preventDefault();
        index = Math.max(0, games.length - 1);
        render();
        break;
      case "Enter":
        event.preventDefault();
        launch();
        break;
      case "Escape":
        event.preventDefault();
        close();
        break;
    }
  }

  function onCarouselClick(event: MouseEvent): void {
    const target = (event.target as HTMLElement).closest<HTMLElement>(".big-item");
    if (!target) {
      return;
    }
    const next = Number(target.dataset.index);
    if (Number.isInteger(next) && next >= 0 && next < games.length) {
      index = next;
      render();
    }
  }

  function onCarouselDoubleClick(event: MouseEvent): void {
    const target = (event.target as HTMLElement).closest<HTMLElement>(".big-item");
    if (target) {
      launch();
    }
  }

  function close(): void {
    if (!active) {
      return;
    }
    active = false;
    root.classList.remove("is-open");
    root.setAttribute("aria-hidden", "true");
    video.replaceChildren();
    video.hidden = true;
    window.clearInterval(padTimer);
    padTimer = 0;
    document.removeEventListener("keydown", onKeyDown);
    if (restoreWindow) {
      changingFullscreen = true;
      void Window.UnFullscreen().catch(onError).finally(() => {
        changingFullscreen = false;
      });
    }
    document.title = "Playnux";
    onExit?.();
  }

  async function open(nextGames: Game[], selectedID = ""): Promise<void> {
    if (active || changingFullscreen) {
      return;
    }
    changingFullscreen = true;
    try {
      restoreWindow = !(await Window.IsFullscreen());
      if (restoreWindow) {
        await Window.Fullscreen();
      }
    } finally {
      changingFullscreen = false;
    }
    games = [...nextGames];
    const found = selectedID ? games.findIndex((game) => game.id === selectedID) : -1;
    index = found >= 0 ? found : 0;
    active = true;
    lastPad = { ...EMPTY_PAD };
    root.classList.add("is-open");
    root.setAttribute("aria-hidden", "false");
    document.addEventListener("keydown", onKeyDown);
    window.clearInterval(padTimer);
    padTimer = window.setInterval(pollGamepad, 140);
    render();
  }

  function setGames(nextGames: Game[]): void {
    const currentID = selected()?.id || "";
    games = [...nextGames];
    const found = currentID ? games.findIndex((game) => game.id === currentID) : -1;
    index = found >= 0 ? found : 0;
    if (active) {
      render();
    }
  }

  closeButton.addEventListener("click", close);
  play.addEventListener("click", launch);
  carousel.addEventListener("click", onCarouselClick);
  carousel.addEventListener("dblclick", onCarouselDoubleClick);

  return {
    open,
    close,
    isOpen: () => active,
    setGames,
  };
}
