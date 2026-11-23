import { t } from "./i18n";
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
import { backgroundTrailer } from "./media";
import { setArtworkImage } from "./artwork-images";

export interface BigPictureOptions {
  root: HTMLElement;
  trailersEnabled: () => boolean;
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
  x: boolean;
  previousPage: boolean;
  nextPage: boolean;
}

const EMPTY_PAD: PadState = {
  left: false,
  right: false,
  up: false,
  down: false,
  a: false,
  b: false,
  x: false,
  previousPage: false,
  nextPage: false,
};

function element<T extends HTMLElement>(root: ParentNode, selector: string): T {
  const found = root.querySelector<T>(selector);
  if (!found) {
    throw new Error(t("Big Picture element {0} is missing", selector));
  }
  return found;
}

export function createBigPicture(options: BigPictureOptions): BigPictureController {
  const { root, onLaunch, onExit, onError, trailersEnabled } = options;
  const background = element<HTMLImageElement>(root, "#big-bg");
  const logo = element<HTMLImageElement>(root, "#big-logo");
  const title = element<HTMLElement>(root, "#big-title");
  const meta = element<HTMLElement>(root, "#big-meta");
  const source = element<HTMLElement>(root, "#big-source");
  const play = element<HTMLButtonElement>(root, "#big-play");
  const sound = element<HTMLButtonElement>(root, "#big-trailer-sound");
  const video = backgroundTrailer(element(root, "#big-video"), sound, true);
  const carousel = element<HTMLElement>(root, "#big-carousel");
  const position = element<HTMLElement>(root, "#big-position");
  const closeButton = element<HTMLButtonElement>(root, "#big-close");


  let active = false;
  let changingFullscreen = false;
  let restoreWindow = false;
  let games: Game[] = [];
  let index = 0;
  let padTimer = 0;
  let lastPad: PadState = { ...EMPTY_PAD };
  let highlighted: HTMLElement | null = null;
  let restoreFocus: HTMLElement | null = null;

  function selected(): Game | null {
    return games[index] || null;
  }

  function updateTrailer(): void {
    video.set(selected(), active && trailersEnabled());
  }

  document.addEventListener("visibilitychange", updateTrailer);
  document.addEventListener("playnux:media-visibility", updateTrailer);

  function renderCarousel(): void {
    const items = games.map((game, itemIndex) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "big-item";
      button.dataset.index = String(itemIndex);
      button.setAttribute("aria-label", game.name);
      button.tabIndex = -1;

      const cover = document.createElement("div");
      cover.className = "big-item-cover";
      cover.textContent = initials(game.name);
      if (game.cover_image) {
        const image = document.createElement("img");
        image.alt = "";
        image.loading = "lazy";
        image.decoding = "async";
        image.addEventListener("load", () => image.classList.add("is-loaded"), { once: true });
        setArtworkImage(image, game.cover_image, devicePixelRatio > 2 ? "carousel-hd" : "carousel");
        cover.append(image);
      }
      const label = document.createElement("span");
      label.textContent = game.name;
      button.append(cover, label);
      return button;
    });
    carousel.replaceChildren(...items);
    highlighted = null;
  }

  function highlight(): void {
    const item = carousel.children[index] as HTMLElement | undefined;
    if (!item || item === highlighted) return;
    const moveFocus = document.activeElement === highlighted;
    highlighted?.classList.remove("is-selected");
    highlighted?.removeAttribute("aria-current");
    if (highlighted) highlighted.tabIndex = -1;
    item.classList.add("is-selected");
    item.setAttribute("aria-current", "true");
    item.tabIndex = 0;
    highlighted = item;
    position.textContent = `${index + 1} / ${games.length}`;
    item.scrollIntoView({ behavior: "instant", block: "nearest", inline: "center" });
    if (moveFocus) item.focus({ preventScroll: true });
  }

  function render(): void {
    const game = selected();
    if (!game) {
      title.textContent = t("Your library is empty");
      meta.textContent = "";
      source.textContent = "";
      play.disabled = true;
      position.textContent = "";
      background.removeAttribute("src");
      logo.hidden = true;
      video.stop();
      return;
    }

    document.title = `${game.name} — Playnux Fullscreen`;
    const backdrop = game.background_image || game.cover_image || "";
    if (backdrop) {
      setArtworkImage(background, backdrop, "big-banner");
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
      setArtworkImage(logo, game.logo_image, "logo-hd");
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
    play.title = canLaunch(game) ? t("Play {0}", game.name) : launchBlockReason(game);
    play.textContent = canLaunch(game) ? t("▶  PLAY") : t("UNAVAILABLE");

    updateTrailer();
    highlight();
  }

  function move(delta: number): void {
    if (games.length === 0) {
      return;
    }
    index = (index + delta + games.length) % games.length;
    render();
  }

  function movePage(direction: number): void {
    const itemWidth = (highlighted?.offsetWidth ?? 105) + 12;
    const step = Math.max(1, Math.floor(carousel.clientWidth / itemWidth) - 1);
    index = Math.max(0, Math.min(games.length - 1, index + direction * step));
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
      x: pressed(2),
      previousPage: pressed(4),
      nextPage: pressed(5),
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
    if (current.previousPage && !lastPad.previousPage) movePage(-1);
    if (current.nextPage && !lastPad.nextPage) movePage(1);
    if (current.x && !lastPad.x && !sound.hidden) sound.click();
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
      case "Tab": {
        const controls = [sound, closeButton, play, highlighted].filter((control): control is HTMLElement =>
          Boolean(control && !control.hidden && !(control instanceof HTMLButtonElement && control.disabled)));
        const current = controls.indexOf(document.activeElement as HTMLElement);
        const offset = event.shiftKey ? -1 : 1;
        const next = current < 0 ? (event.shiftKey ? controls.length - 1 : 0) : (current + offset + controls.length) % controls.length;
        event.preventDefault();
        controls[next]?.focus({ preventScroll: true });
        break;
      }
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
      case "PageUp":
      case "PageDown":
        event.preventDefault();
        movePage(event.key === "PageUp" ? -1 : 1);
        break;
      case "Enter":
        if (event.target instanceof HTMLButtonElement && root.contains(event.target) && !carousel.contains(event.target)) break;
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
    carousel.replaceChildren();
    highlighted = null;
    root.classList.remove("is-open");
    root.setAttribute("aria-hidden", "true");
    video.stop();
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
    restoreFocus?.focus({ preventScroll: true });
    restoreFocus = null;
    onExit?.();
  }

  async function open(nextGames: Game[], selectedID = ""): Promise<void> {
    if (active || changingFullscreen) {
      return;
    }
    restoreFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
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
    renderCarousel();
    render();
    (highlighted ?? closeButton).focus({ preventScroll: true });
  }

  function setGames(nextGames: Game[]): void {
    const currentID = selected()?.id || "";
    games = [...nextGames];
    const found = currentID ? games.findIndex((game) => game.id === currentID) : -1;
    index = found >= 0 ? found : 0;
    if (active) {
      const focusedGame = carousel.contains(document.activeElement);
      renderCarousel();
      render();
      if (focusedGame) (highlighted ?? closeButton).focus({ preventScroll: true });
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
