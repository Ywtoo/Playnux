import { t } from "../../i18n";
import * as Host from "../../../bindings/github.com/playnux/ui/host/host.js";
import { bindDirectoryPicker, button, type SettingsContext } from "../settings";
import "./settings.css";

export function setup(root: HTMLElement, context: SettingsContext): void {
  const field = <T extends HTMLElement>(id: string) => root.querySelector<T>(`#${id}`)!;
  const folder = field<HTMLInputElement>("emulation-folder");
  const profile = field<HTMLSelectElement>("emulation-profile");
  const scan = field<HTMLButtonElement>("emulation-scan");
  const folderStatus = field("emulation-status");
  const emulatorStatus = field("emulator-status");
  const form = field<HTMLFormElement>("emulator-form");
  const automatic = "automatic";
  const unavailableMessage = t("Scanning is unavailable. Configure an emulator with a ROM profile.");
  bindDirectoryPicker(field("emulation-browse"), folder, context);

  async function render(): Promise<void> {
    const [catalog, folders, mappings] = await Promise.all([
      Host.GetEmulators(),
      Host.GetRomFolders(),
      Host.GetEmulationFolderProfiles(),
    ]);
    const list = field("emulation-list");
    const previous = profile.value;
    const profiles = new Map<string, string>();
    const cards = (catalog ?? []).map((emulator) => {
      const card = document.createElement("article");
      card.className = "emulation-card";
      const heading = document.createElement("div");
      heading.className = "emulation-card-head";
      const name = document.createElement("strong");
      name.textContent = emulator.name;
      const badge = document.createElement("span");
      badge.className = "emulation-badge";
      badge.textContent = emulator.custom ? "Manual" : t("Detected");
      heading.append(name, badge);
      if (emulator.custom) {
        heading.append(button(t("Remove"), () => {
          if (!window.confirm(t("Remove the {0} emulator?", emulator.name))) return;
          void context.run(async () => {
            await Host.RemoveEmulator(emulator.id);
            await render();
            context.status.textContent = t("Emulator removed.");
          });
        }));
      }
      card.append(heading);
      const details = document.createElement("p");
      details.className = "emulation-profiles";
      details.textContent = (emulator.profiles ?? []).map((item) => {
        if (item.executable || item.flatpak_id) {
          profiles.set(item.profile_id, `${emulator.name} · ${item.name} (${(item.extensions ?? []).join(", ")})`);
        }
        return item.name;
      }).join(" · ") || t("Add a profile to scan ROMs with this emulator.");
      card.append(details);
      return card;
    });
    list.replaceChildren(...cards);
    if (!cards.length) list.textContent = t("No emulator detected. You can add one below.");

    const entries = (folders ?? []).map((path) => {
      const profileID = mappings?.[path] ?? "";
      const row = document.createElement("div");
      row.className = "emulation-folder-row";
      const label = document.createElement("div");
      label.className = "emulation-folder-label";
      const name = document.createElement("span");
      name.textContent = path;
      const mode = document.createElement("small");
      mode.textContent = profiles.get(profileID) ?? (profileID ? t("Profile unavailable") : t("Automatic"));
      label.append(name, mode);
      const edit = button(t("Edit profile"), () => {
        folder.value = path;
        profile.value = profileID ? (profiles.has(profileID) ? profileID : "") : automatic;
        profile.focus();
      });
      const remove = button(t("Remove"), () => void context.run(async () => {
        await Host.RemoveRomFolder(path);
        await render();
        await context.reload();
        folderStatus.textContent = t("Folder removed.");
      }, folderStatus));
      row.append(label, edit, remove);
      return row;
    });
    const title = document.createElement("h4");
    title.textContent = t("Configured ROM folders");
    field("emulation-folders").replaceChildren(...(entries.length ? [title, ...entries] : []));

    profile.replaceChildren(new Option(profiles.size ? t("Choose a profile") : t("No profile available"), ""));
    if (profiles.size) {
      profile.add(
        new Option(t("Automatic (folder and extension)"), automatic),
      );
      for (const [id, label] of profiles) profile.add(new Option(label, id));
    }
    if (previous === automatic && profiles.size || profiles.has(previous)) {
      profile.value = previous;
    } else if (profiles.size === 1) {
      profile.value = profiles.keys().next().value!;
    }
    profile.disabled = scan.disabled = !profiles.size;
    if (!profiles.size) folderStatus.textContent = unavailableMessage;
    else if (folderStatus.textContent === unavailableMessage) folderStatus.textContent = "";
  }

  field("emulation-scan-form").addEventListener("submit", (event) => {
    event.preventDefault();
    if (scan.disabled || !folder.value.trim() || !profile.value) return;
    void context.run(async () => {
      folderStatus.textContent = t("Scanning ROMs…");
      const games = await Host.AddEmulationFolder(folder.value.trim(), profile.value === automatic ? "" : profile.value);
      await render();
      await context.reload(true);
      const count = games?.length ?? 0;
      folderStatus.textContent = count
        ? t("Folder saved. {0} ROMs found in emulation folders.", count)
        : t("Folder saved. Check that the ROM extensions match the selected profile.");
    }, folderStatus);
  });

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const value = (id: string) => field<HTMLInputElement>(id).value.trim();
    const name = value("emulator-name");
    const executable = value("emulator-executable");
    const flatpakID = value("emulator-flatpak");
    const extensions = value("emulator-extensions").split(/[\s,;]+/).filter(Boolean);
    if ((!executable && !flatpakID) || !extensions.length) {
      emulatorStatus.textContent = t("Enter the extensions and the executable or Flatpak ID.");
      return;
    }
    const id = name.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9._-]+/g, "-").replace(/^-+|-+$/g, "") || "custom-emulator";
    void context.run(async () => {
      await Host.SaveEmulator({
        id, name, executable, flatpak_id: flatpakID,
        profiles: [{
          profile_id: `${id}:default`,
          emulator_id: id,
          name: value("emulator-profile-name"),
          platforms: [value("emulator-platform")],
          extensions,
          executable,
          flatpak_id: flatpakID,
          args_template: value("emulator-args"),
          folder_aliases: [],
          priority: 100,
          builtin: false,
        }],
      });
      form.reset();
      await render();
      profile.value = `${id}:default`;
      emulatorStatus.textContent = t("Emulator saved. Choose a ROM folder for this profile.");
    }, emulatorStatus);
  });

  void render().catch((error) => { folderStatus.textContent = context.error(error); });
}
