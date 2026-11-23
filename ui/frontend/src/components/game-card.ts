import { t } from "../i18n";
import type { Game } from "../../bindings/github.com/playnux/core/models/models";
import { setArtworkImage } from "../artwork-images";
import { canLaunch, formatPlaytime, initials, launchBlockReason, sourceClass, sourceLabel } from "../format";

// Cartão da grade. Usa light DOM de propósito: o CSS global já estiliza a
// biblioteca e o card continua acessível para os testes de layout.
export class PlaynuxGameCardElement extends HTMLElement {
  static get observedAttributes(): string[] {
    return [];
  }

  private gameValue: Game | null = null;
  private readonly frame: HTMLElement;
  private readonly cover: HTMLElement;
  private readonly image: HTMLImageElement;
  private readonly fallback: HTMLElement;
  private readonly fallbackText: HTMLElement;
  private readonly coverShade: HTMLElement;
  private readonly badge: HTMLElement;
  private readonly playButton: HTMLButtonElement;
  private readonly heading: HTMLElement;
  private readonly icon: HTMLImageElement;
  private readonly source: HTMLElement;
  private readonly playtime: HTMLElement;
  private readonly sourceDot: HTMLElement;
  private readonly installedPip: HTMLElement;
  private lastCover = "";

  constructor() {
    super();

    this.frame = document.createElement("article");
    this.frame.className = "game-card";
    this.frame.tabIndex = 0;
    this.frame.setAttribute("role", "group");

    this.cover = document.createElement("div");
    this.cover.className = "game-cover";

    this.image = document.createElement("img");
    this.image.alt = "";
    this.image.loading = "lazy";
    this.image.decoding = "async";
    this.image.draggable = false;

    this.fallback = document.createElement("div");
    this.fallback.className = "cover-fallback";
    this.fallback.setAttribute("aria-hidden", "true");
    this.fallbackText = document.createElement("span");
    this.fallback.appendChild(this.fallbackText);

    this.coverShade = document.createElement("div");
    this.coverShade.className = "game-shade";

    this.badge = document.createElement("span");
    this.badge.className = "game-badge";

    this.playButton = document.createElement("button");
    this.playButton.className = "game-play";
    this.playButton.type = "button";
    this.playButton.textContent = "▶";
    this.playButton.setAttribute("aria-label", t("Play"));

    this.cover.append(this.image, this.fallback, this.coverShade, this.badge, this.playButton);

    const body = document.createElement("div");
    body.className = "game-body";

    this.heading = document.createElement("h3");
    this.heading.className = "game-title";

    const subtitle = document.createElement("div");
    subtitle.className = "game-sub";
    this.source = document.createElement("span");
    this.source.className = "game-source";
    this.playtime = document.createElement("span");
    this.playtime.className = "game-playtime";

    this.sourceDot = document.createElement("span");
    this.sourceDot.className = "source-dot";
    this.sourceDot.setAttribute("aria-hidden", "true");

    this.installedPip = document.createElement("span");
    this.installedPip.className = "installed-pip";
    this.installedPip.title = t("Installed");
    this.installedPip.setAttribute("role", "img");

    subtitle.append(this.sourceDot, this.source, this.playtime, this.installedPip);
    this.icon = document.createElement("img");
    this.icon.className = "game-library-icon";
    this.icon.alt = "";
    this.icon.loading = "lazy";
    this.icon.decoding = "async";
    this.icon.hidden = true;
    this.icon.addEventListener("error", () => { this.icon.hidden = true; });
    const titleRow = document.createElement("div");
    titleRow.className = "game-title-row";
    titleRow.append(this.icon, this.heading);
    body.append(titleRow, subtitle);
    this.frame.append(this.cover, body);

    this.image.addEventListener("load", () => {
      this.image.classList.remove("is-missing");
      this.fallback.classList.add("is-hidden");
      this.cover.classList.remove("is-loading");
    });
    this.image.addEventListener("error", () => {
      this.image.classList.add("is-missing");
      this.fallback.classList.remove("is-hidden");
      this.cover.classList.remove("is-loading");
    });

    this.frame.addEventListener("click", () => this.open());
    this.frame.addEventListener("keydown", (event) => {
      if (event.target !== this.frame) {
        return;
      }
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        this.open();
      }
    });
    this.playButton.addEventListener("click", (event) => {
      event.stopPropagation();
      if (this.gameValue) {
        this.dispatchEvent(
          new CustomEvent<Game>("playnux:play", {
            detail: this.gameValue,
            bubbles: true,
            composed: true,
          }),
        );
      }
    });
  }

  connectedCallback(): void {
    if (!this.frame.parentNode) this.appendChild(this.frame);
    this.render();
  }

  set game(value: Game | null) {
    this.gameValue = value;
    if (this.isConnected) {
      this.render();
    }
  }

  get game(): Game | null {
    return this.gameValue;
  }

  private open(): void {
    if (!this.gameValue) {
      return;
    }
    this.dispatchEvent(
      new CustomEvent<Game>("playnux:open", {
        detail: this.gameValue,
        bubbles: true,
        composed: true,
      }),
    );
  }

  private render(): void {
    const game = this.gameValue;
    if (!game) {
      this.frame.hidden = true;
      return;
    }
    this.frame.hidden = false;
    this.frame.classList.toggle("is-uninstalled", !game.is_installed);
    this.frame.style.setProperty("--custom-contextmenu-data", game.id);
    this.heading.textContent = game.name;
    const icon = game.icon || "";
    if (/^(https?:|data:image\/|\/artwork\/)/i.test(icon)) {
      this.icon.hidden = false;
      setArtworkImage(this.icon, icon, "icon");
    } else {
      this.icon.hidden = true;
      this.icon.removeAttribute("src");
    }
    this.source.textContent = sourceLabel(game);
    this.playtime.textContent = formatPlaytime(game.playtime);
    this.playtime.hidden = !this.playtime.textContent;
    const installed = game.is_installed ? t("Installed") : t("Not installed");
    this.installedPip.title = installed;
    this.installedPip.setAttribute("aria-label", installed);

    this.sourceDot.className = `source-dot ${sourceClass(game)}`;
    this.badge.textContent = sourceLabel(game);

    const launchable = canLaunch(game);
    this.playButton.disabled = game.is_installed && !launchable;
    if (!game.is_installed) {
      this.playButton.textContent = "⬇";
      this.playButton.title = t("Install {0}", game.name);
      this.playButton.setAttribute("aria-label", t("Install {0}", game.name));
    } else {
      this.playButton.textContent = "▶";
      this.playButton.title = launchable
        ? t("Play {0}", game.name)
        : launchBlockReason(game);
      this.playButton.setAttribute("aria-label", t("Play {0}", game.name));
    }
    this.frame.setAttribute("aria-label", t("Open details for {0}", game.name));

    this.fallbackText.textContent = initials(game.name);
    if (game.cover_image !== this.lastCover) {
      this.lastCover = game.cover_image;
      this.cover.classList.toggle("is-loading", Boolean(game.cover_image));
      this.fallback.classList.remove("is-hidden");
      this.image.classList.add("is-missing");
      if (!game.cover_image) {
        this.image.removeAttribute("src");
      } else {
        setArtworkImage(this.image, game.cover_image, "cover");
      }
    }
  }
}

if (!customElements.get("playnux-game-card")) {
  customElements.define("playnux-game-card", PlaynuxGameCardElement);
}
