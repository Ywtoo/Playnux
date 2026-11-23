import { t } from "../i18n";
import * as Host from "../../bindings/github.com/playnux/ui/host/host.js";
import type { SettingsContext } from "./settings";

export function setupStoreLogin(root: HTMLElement, context: SettingsContext): void {
  root.querySelector("[data-store-login]")?.addEventListener("click", () => void context.run(async () => {
    context.status.textContent = t("Connecting to {0}…", context.store.name);
    let games;
    try {
      games = await Host.LoginStore(context.store.id);
    } finally {
      await context.reload(true);
    }
    context.status.textContent = t("Account connected. {0} games imported.", games?.length ?? 0);
  }));
}
