import type { Game } from "../../bindings/github.com/playnux/core/models/models";
import { canLaunch, cardSubtitle, initials, launchBlockReason, sourceClass, sourceLabel } from "../format";

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
  private readonly subtitle: HTMLElement;
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
    this.playButton.setAttribute("aria-label", "Jogar");

    this.cover.append(this.image, this.fallback, this.coverShade, this.badge, this.playButton);

    const body = document.createElement("div");
    body.className = "game-body";

    this.heading = document.createElement("h3");
    this.heading.className = "game-title";

    this.subtitle = document.createElement("div");
    this.subtitle.className = "game-sub";

    this.sourceDot = document.createElement("span");
    this.sourceDot.className = "source-dot";
    this.sourceDot.setAttribute("aria-hidden", "true");

    this.installedPip = document.createElement("span");
    this.installedPip.className = "installed-pip";
    this.installedPip.title = "Instalado";
    this.installedPip.hidden = true;

    this.subtitle.append(this.sourceDot, document.createTextNode(""), this.installedPip);
    this.icon = document.createElement("img");
    this.icon.className = "game-library-icon";
    this.icon.alt = "";
    this.icon.hidden = true;
    this.icon.addEventListener("error", () => { this.icon.hidden = true; });
    const titleRow = document.createElement("div");
    titleRow.className = "game-title-row";
    titleRow.append(this.icon, this.heading);
    body.append(titleRow, this.subtitle);
    this.frame.append(this.cover, body);

    this.image.addEventListener("load", () => {
      this.image.classList.remove("is-missing");
      this.fallback.classList.add("is-hidden");
    });
    this.image.addEventListener("error", () => {
      this.image.classList.add("is-missing");
      this.fallback.classList.remove("is-hidden");
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
      this.icon.src = icon;
    } else {
      this.icon.hidden = true;
      this.icon.removeAttribute("src");
    }
    this.subtitle.childNodes[1].textContent = cardSubtitle(game);
    this.installedPip.hidden = !game.is_installed;

    this.sourceDot.className = `source-dot ${sourceClass(game)}`;
    this.badge.textContent = sourceLabel(game);

    const launchable = canLaunch(game);
    this.playButton.disabled = false;
    if (!game.is_installed) {
      this.playButton.textContent = "⬇";
      this.playButton.title = `Instalar ${game.name}`;
      this.playButton.setAttribute("aria-label", `Instalar ${game.name}`);
    } else {
      this.playButton.textContent = "▶";
      this.playButton.disabled = !launchable;
      this.playButton.title = launchable
        ? `Jogar ${game.name}`
        : launchBlockReason(game);
      this.playButton.setAttribute("aria-label", `Jogar ${game.name}`);
    }
    this.frame.setAttribute("aria-label", `Abrir detalhes de ${game.name}`);

    this.fallbackText.textContent = initials(game.name);
    if (game.cover_image !== this.lastCover) {
      this.lastCover = game.cover_image;
      if (!game.cover_image) {
        this.image.removeAttribute("src");
        this.image.classList.add("is-missing");
        this.fallback.classList.remove("is-hidden");
      } else {
        this.image.src = game.cover_image;
      }
    }
  }
}

if (!customElements.get("playnux-game-card")) {
  customElements.define("playnux-game-card", PlaynuxGameCardElement);
}
