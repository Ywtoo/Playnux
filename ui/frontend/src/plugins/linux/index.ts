import { t } from "../../i18n";
import * as Host from "../../../bindings/github.com/playnux/ui/host/host.js";
import { bindDirectoryPicker, button, type SettingsContext } from "../settings";

export function setup(root: HTMLElement, context: SettingsContext): void {
  const form = root.querySelector<HTMLFormElement>("form")!;
  const path = form.querySelector<HTMLInputElement>("input")!;
  const list = form.querySelector(".folder-list")!;
  bindDirectoryPicker(form.querySelector("[data-browse]")!, path, context);

  async function renderFolders(): Promise<void> {
    const folders = await Host.GetLocalFolders();
    list.replaceChildren(...(folders ?? []).map((folder) => {
      const row = document.createElement("div");
      row.className = "folder-row";
      const label = document.createElement("span");
      label.textContent = folder;
      label.title = folder;
      const remove = button(t("Remove"), () => void context.run(async () => {
        await Host.RemoveLocalFolder(folder);
        await renderFolders();
        await context.reload();
        context.status.textContent = t("Folder removed.");
      }));
      row.append(label, remove);
      return row;
    }));
  }

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const folder = path.value.trim();
    if (!folder) return;
    void context.run(async () => {
      context.status.textContent = t("Scanning game folders…");
      const games = await Host.AddLocalFolder(folder);
      form.reset();
      await renderFolders();
      await context.reload(true);
      context.status.textContent = t("Folder saved. Found {0} games in this local source.", games?.length ?? 0);
    });
  });

  void renderFolders().catch((error) => {
    if (form.isConnected) context.status.textContent = context.error(error);
  });
}
