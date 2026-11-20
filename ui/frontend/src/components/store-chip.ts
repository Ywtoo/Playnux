import type { StoreInfo } from "../../bindings/github.com/playnux/core/app/models";

// Chip de loja da barra lateral. O clique não conhece o Core: apenas anuncia a
// loja escolhida para o main.ts decidir entre login e sincronização.
export class PlaynuxStoreChipElement extends HTMLElement {
  private storeValue: StoreInfo | null = null;
  private busyValue = false;
  private readonly button: HTMLButtonElement;
  private readonly dot: HTMLElement;
  private readonly name: HTMLElement;
  private readonly action: HTMLElement;

  constructor() {
    super();

    this.button = document.createElement("button");
    this.button.type = "button";
    this.button.className = "store-chip";

    this.dot = document.createElement("span");
    this.dot.className = "store-dot";
    this.dot.setAttribute("aria-hidden", "true");

    this.name = document.createElement("span");
    this.name.className = "store-name";

    this.action = document.createElement("span");
    this.action.className = "store-action";

    this.button.append(this.dot, this.name, this.action);

    this.button.addEventListener("click", () => {
      if (!this.storeValue || this.busyValue) {
        return;
      }
      this.dispatchEvent(
        new CustomEvent<string>("playnux:store", {
          detail: this.storeValue.id,
          bubbles: true,
          composed: true,
        }),
      );
    });
  }

  connectedCallback(): void {
    if (!this.button.parentNode) this.appendChild(this.button);
    this.render();
  }

  set store(value: StoreInfo | null) {
    this.storeValue = value;
    if (this.isConnected) {
      this.render();
    }
  }

  get store(): StoreInfo | null {
    return this.storeValue;
  }

  set busy(value: boolean) {
    this.busyValue = value;
    if (this.isConnected) {
      this.render();
    }
  }

  private render(): void {
    const store = this.storeValue;
    if (!store) {
      this.button.hidden = true;
      return;
    }
    this.button.hidden = false;
    this.name.textContent = store.name;
    this.action.textContent = this.busyValue ? "…" : store.connected ? "Sincronizar" : "Conectar";
    this.button.disabled = this.busyValue;
    this.button.classList.toggle("is-connected", store.connected);
    this.button.setAttribute("aria-busy", String(this.busyValue));
    this.button.setAttribute(
      "aria-label",
      `${store.connected ? "Sincronizar" : "Conectar"} ${store.name}`,
    );
    this.button.title = store.connected
      ? `${store.name} conectada — sincronizar biblioteca`
      : `Entrar na ${store.name}`;
  }
}

if (!customElements.get("playnux-store-chip")) {
  customElements.define("playnux-store-chip", PlaynuxStoreChipElement);
}
