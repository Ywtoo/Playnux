import { Browser } from "@wailsio/runtime";
import * as Host from "../bindings/github.com/playnux/ui/host/host.js";
import type { Update } from "../bindings/github.com/playnux/ui/internal/release/models.js";
import { t } from "./i18n";

export function setupUpdates(report: (message: string, error?: boolean) => void): () => void {
  const dialog = document.querySelector<HTMLDialogElement>("#update-dialog")!;
  const status = document.querySelector<HTMLElement>("#update-status")!;
  const checkButton = document.querySelector<HTMLButtonElement>("#update-check")!;
  const installButton = document.querySelector<HTMLButtonElement>("#update-install")!;
  let result: Update | undefined;
  let installed = false;
  let busy = false;

  function render(): void {
    installButton.hidden = !installed && !result?.available;
    installButton.textContent = installed ? t("Restart Playnux") : result?.installable ? t("Install update") : t("View release");
    checkButton.disabled = busy || installed;
    installButton.disabled = busy;
  }

  async function check(interactive: boolean): Promise<void> {
    if (interactive && !dialog.open) dialog.showModal();
    if (busy || installed) return;
    busy = true;
    result = undefined;
    if (interactive) status.textContent = t("Checking for updates…");
    render();
    try {
      result = await Host.CheckForUpdates();
      if (result.available) {
        status.textContent = t("Playnux {0} is available. You are running {1}.", result.latest, result.current);
        if (!result.installable) status.textContent += ` ${t("Use the release page to update this installation.")}`;
        if (!interactive) {
          if (document.querySelector("dialog[open]")) report(t("A Playnux update is available in the application menu."));
          else if (!dialog.open) dialog.showModal();
        }
      } else {
        status.textContent = result.latest
          ? t("Playnux {0} is up to date.", result.current)
          : t("No published releases found. Installed version: {0}.", result.current);
      }
    } catch (error) {
      status.textContent = t("Could not check for updates. {0}", error instanceof Error ? error.message : String(error));
    } finally {
      busy = false;
      render();
    }
  }

  checkButton.addEventListener("click", () => void check(true));
  document.querySelector("#update-close")!.addEventListener("click", () => dialog.close());
  installButton.addEventListener("click", () => {
    if (busy || !result?.available) return;
    if (!result.installable) {
      void Browser.OpenURL(result.url).catch((error) => report(String(error), true));
      return;
    }
    busy = true;
    render();
    if (!installed) status.textContent = t("Downloading and verifying the update…");
    const action = installed ? Host.RestartAfterUpdate() : Host.InstallUpdate(result.latest);
    void action.then(() => {
      installed = true;
      status.textContent = t("Update installed. Restart Playnux to finish.");
    }).catch((error) => {
      status.textContent = t("Could not complete the update. {0}", error instanceof Error ? error.message : String(error));
    }).finally(() => {
      busy = false;
      render();
    });
  });

  window.setTimeout(() => void check(false), 1800);
  return () => void check(true);
}
