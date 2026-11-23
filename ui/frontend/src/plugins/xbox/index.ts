import { t } from "../../i18n";
import { setupStoreLogin } from "../store-login";
import type { SettingsContext } from "../settings";

export function setup(root: HTMLElement, context: SettingsContext): void {
  const reason = context.store.login_unavailable_reason;
  if (reason) {
    const message = root.querySelector<HTMLElement>("[data-xbox-unavailable]")!;
    message.textContent = t(reason);
    message.hidden = false;
    root.querySelector<HTMLElement>("[data-xbox-help]")!.hidden = true;
    root.querySelector<HTMLButtonElement>("[data-store-login]")!.hidden = true;
    return;
  }
  setupStoreLogin(root, context);
}
