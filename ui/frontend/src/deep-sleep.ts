import { Events } from "@wailsio/runtime";
import type { Game } from "../bindings/github.com/playnux/core/models/models";

// O Go congela o heap e libera as capas; o lado web apenas troca o que está
// sendo exibido. As imagens são substituídas por um pixel transparente para
// soltar as texturas decodificadas, e restauradas ao acordar.
const TRANSPARENT_PIXEL =
  "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7";

const SELECTOR = [
  ".game-cover img",
  ".details-row-thumb img",
  ".big-item-cover img",
  "#drawer-cover",
  "#details-bg",
  "#details-logo",
  "#big-bg",
  "#big-logo",
].join(", ");

let sleeping = false;
let overlayTitle: HTMLElement | null = null;
let defaultTitle = document.title;

declare global {
  interface Window {
    playnuxSleep?: () => void;
    playnuxWake?: () => void;
  }
}

function titleElement(): HTMLElement | null {
  if (!overlayTitle) {
    overlayTitle = document.getElementById("sleep-game");
  }
  return overlayTitle;
}

function purgeImages(): void {
  document.querySelectorAll<HTMLImageElement>(SELECTOR).forEach((image) => {
    if (image.dataset.coverSrc) {
      image.src = TRANSPARENT_PIXEL;
      return;
    }
    const source = image.currentSrc || image.src || image.getAttribute("src") || "";
    if (!source || source === TRANSPARENT_PIXEL) {
      return;
    }
    image.dataset.coverSrc = source;
    image.src = TRANSPARENT_PIXEL;
  });
}

function restoreImages(): void {
  document.querySelectorAll<HTMLImageElement>(SELECTOR).forEach((image) => {
    const source = image.dataset.coverSrc;
    if (!source) {
      return;
    }
    delete image.dataset.coverSrc;
    image.src = source;
  });
}

function gameFromEvent(value: unknown): Game | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const name = (value as { name?: unknown }).name;
  return typeof name === "string" && name ? (value as Game) : null;
}

export function enterSleep(game: Game | null): void {
  if (!sleeping) {
    sleeping = true;
    document.documentElement.dataset.deepSleep = "true";
    defaultTitle = document.title;
    purgeImages();
  }
  const label = game && game.name ? game.name : "Jogo em execução";
  const title = titleElement();
  if (title) {
    title.textContent = label;
  }
  document.title = `${label} — Playnux`;
}

export function leaveSleep(): void {
  if (!sleeping) {
    return;
  }
  sleeping = false;
  restoreImages();
  delete document.documentElement.dataset.deepSleep;
  document.title = defaultTitle;
}

export function installDeepSleep(): void {
  window.playnuxSleep = () => enterSleep(null);
  window.playnuxWake = () => leaveSleep();

  Events.On("gameplay:sleep", (event) => {
    enterSleep(gameFromEvent(event.data));
  });
  Events.On("gameplay:wake", () => {
    leaveSleep();
  });
}
