import { Browser } from "@wailsio/runtime";
import * as Host from "../bindings/github.com/playnux/ui/host/host.js";
import type { Game } from "../bindings/github.com/playnux/core/models/models.js";
import type { LibrarySettings, MetadataSettings, StoreInfo } from "../bindings/github.com/playnux/core/app/models.js";

import { applyUISettings, type UISettingsLike } from "./appearance";
import { createBackgroundDownloads } from "./background-downloads";
import { libraryPlugins, metadataPlugins } from "./plugins";

type MetadataPluginID = keyof typeof metadataPlugins;
type Panel = "stores" | "library" | "plugins" | "emulation" | "appearance" | MetadataPluginID;

interface EmulatorProfile {
  profile_id: string;
  emulator_id: string;
  name: string;
  platforms: string[];
  extensions: string[];
  executable?: string;
  flatpak_id?: string;
  args_template: string;
  builtin?: boolean;
}

interface EmulatorDefinition {
  id: string;
  name: string;
  executable?: string;
  flatpak_id?: string;
  profiles: EmulatorProfile[];
  custom?: boolean;
}

interface Options {
  stores: () => StoreInfo[];
  games: () => Game[];
  filteredGames: () => Game[];
  selected: () => Game | null;
  configureFolders: (kind: "local" | "rom") => Promise<void>;
  addGame: () => void;
  reload: (discoverNew?: boolean) => Promise<void>;
  applyDownloads: (games: Game[]) => void;
  error: (error: unknown) => string;
  report: (message: string, error?: boolean) => void;
  applyAppearance: (settings: UISettingsLike) => void;
  setViewMode: (mode: "details" | "grid") => void;
}

function element<T extends HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (!node) throw new Error(`Elemento ausente: ${id}`);
  return node as T;
}

function button(label: string, action: () => void): HTMLButtonElement {
  const control = document.createElement("button");
  control.className = "ghost-button";
  control.type = "button";
  control.textContent = label;
  control.addEventListener("click", action);
  return control;
}

const metadataSources = {
  store: "Loja oficial do jogo",
  igdb: "IGDB",
  steamgriddb: "SteamGridDB",
};
type MetadataSource = keyof typeof metadataSources;
const defaultPriority = Object.keys(metadataSources) as MetadataSource[];

export function setupIntegrations(options: Options) {
  const dialog = element<HTMLDialogElement>("integrations-dialog");
  const status = element("integrations-status");
  const metadataDialog = element<HTMLDialogElement>("metadata-dialog");
  const metadataStatus = element("metadata-status");
  const storeProperties = element("integration-store-properties");
  const storeActions = element("integration-store-actions");
  const storeName = element("integration-store-name");
  const storeList = element("integration-stores");
  const policyForm = element<HTMLFormElement>("metadata-policy");
  const sourceList = element("metadata-sources");
  const automatic = element<HTMLInputElement>("metadata-auto");
  const storeConfig = element("integration-store-config");
  const providerList = element("integration-providers");
  const content = element("integration-content");
  for (const [id, plugin] of Object.entries(metadataPlugins)) {
    content.insertAdjacentHTML("beforeend", plugin.view);
    const tab = button(plugin.name, () => showPanel(id as MetadataPluginID));
    tab.dataset.integrationTab = id;
    providerList.append(tab);
  }
  const panels = dialog.querySelectorAll<HTMLElement>("[data-integration-panel]");
  const tabs = dialog.querySelectorAll<HTMLButtonElement>("[data-integration-tab]");
  let settings: MetadataSettings | null = null;
  let busy = false;
  let currentPanel: Panel = "stores";
  let activeStore = "";
  let metadataGame: Game | null = null;
  let sourceOrder = [...defaultPriority];
  let activeSources = new Set<MetadataSource>(defaultPriority);
  let libraryPriority: string[] = [];
  let emulationCatalog: EmulatorDefinition[] = [];
  let emulationFolders: Record<string, string> = {};
  const downloads = createBackgroundDownloads(options.error);

  function showPanel(panel: Panel): void {
    currentPanel = panel;
    status.textContent = "";
    renderStores();
    panels.forEach((section) => { section.hidden = section.dataset.integrationPanel !== panel; });
    tabs.forEach((tab) => {
      if (tab.dataset.integrationTab === panel) tab.setAttribute("aria-current", "page");
      else tab.removeAttribute("aria-current");
    });
    dialog.querySelectorAll<HTMLButtonElement>("[data-integration-panel-link]").forEach((link) => {
      link.toggleAttribute("aria-current", link.dataset.integrationPanelLink === panel);
    });
    if (panel === "library") void renderLibrarySettings();
    if (panel === "emulation") void renderEmulation();
    if (panel === "appearance") void renderAppearance();
  }

  function renderLibraryPriority(): void {
    const list = element("library-priority");
    const names = new Map(options.stores().map((store) => [store.id, store.name]));
    list.replaceChildren(...libraryPriority.map((id, index) => {
      const row = document.createElement("li");
      const label = document.createElement("span");
      label.textContent = names.get(id) ?? id;
      row.append(label);
      for (const [step, symbol, description] of [[-1, "↑", "Subir"], [1, "↓", "Descer"]] as const) {
        const target = index + step;
        const move = button(symbol, () => {
          [libraryPriority[index], libraryPriority[target]] = [libraryPriority[target], libraryPriority[index]];
          renderLibraryPriority();
        });
        move.setAttribute("aria-label", `${description} ${names.get(id) ?? id}`);
        move.disabled = target < 0 || target >= libraryPriority.length;
        row.append(move);
      }
      return row;
    }));
  }

  async function renderLibrarySettings(): Promise<void> {
    const statusLine = element("library-status");
    try {
      const settings: LibrarySettings = await Host.GetLibrarySettings();
      element<HTMLInputElement>("library-unify").checked = settings.unify;
      const known = new Set(options.stores().map((store) => store.id));
      libraryPriority = settings.priority.filter((id) => known.has(id));
      for (const id of known) {
        if (!libraryPriority.includes(id)) libraryPriority.push(id);
      }
      renderLibraryPriority();
      statusLine.textContent = "";
    } catch (error) {
      statusLine.textContent = options.error(error);
    }
  }

  async function saveLibrarySettings(): Promise<void> {
    const statusLine = element("library-status");
    statusLine.textContent = "Salvando…";
    try {
      await Host.SaveLibrarySettings({
        unify: element<HTMLInputElement>("library-unify").checked,
        priority: libraryPriority,
      });
      await options.reload(false);
      statusLine.textContent = "Preferências salvas.";
    } catch (error) {
      statusLine.textContent = options.error(error);
    }
  }

  async function clearLibrary(): Promise<void> {
    const statusLine = element("library-status");
    if (!window.confirm("Limpar a biblioteca? Todos os jogos importados e as pastas locais serão apagados. As contas conectadas continuam salvas.")) {
      return;
    }
    statusLine.textContent = "Limpando biblioteca…";
    try {
      await Host.ClearLibrary();
      await options.reload(false);
      await renderLibrarySettings();
      statusLine.textContent = "Biblioteca limpa.";
    } catch (error) {
      statusLine.textContent = options.error(error);
    }
  }

  function updateProviderStates(): void {
    if (!settings) return;
    for (const name of ["igdb", "steamgriddb"] as const) {
      const label = dialog.querySelector(`[data-provider-state="${name}"]`);
      if (label instanceof HTMLElement) {
        const saved = settings.saved_keys?.[name];
        label.textContent = saved ? "Chave cadastrada"
          : settings[name] ? "Chave fornecida pelo ambiente" : "Chave não cadastrada";
        label.classList.add("credential-status");
        label.classList.toggle("is-configured", settings[name]);
      }
    }
  }

  function renderSourceOrder(focusSource?: MetadataSource, focusStep?: number): void {
    sourceList.replaceChildren(...sourceOrder.map((source, index) => {
      const row = document.createElement("li");
      const label = document.createElement("label");
      const toggle = document.createElement("input");
      toggle.type = "checkbox";
      toggle.checked = activeSources.has(source);
      toggle.addEventListener("change", () => {
        if (toggle.checked) activeSources.add(source);
        else activeSources.delete(source);
      });
      label.append(toggle, metadataSources[source]);
      row.append(label);
      for (const [step, symbol, description] of [[-1, "↑", "Subir"], [1, "↓", "Descer"]] as const) {
        const target = index + step;
        const move = button(symbol, () => {
          [sourceOrder[index], sourceOrder[target]] = [sourceOrder[target], sourceOrder[index]];
          renderSourceOrder(source, step);
        });
        move.setAttribute("aria-label", `${description} ${metadataSources[source]}`);
        move.disabled = target < 0 || target >= sourceOrder.length;
        row.append(move);
        if (source === focusSource && step === focusStep) {
          queueMicrotask(() => (move.disabled ? toggle : move).focus());
        }
      }
      return row;
    }));
  }

  function renderPolicy(): void {
    if (!settings) return;
    automatic.checked = settings.policy.automatic;
    const saved = (settings.policy.sources ?? []).filter((id): id is MetadataSource => defaultPriority.includes(id as MetadataSource));
    activeSources = new Set(saved);
    sourceOrder = [...saved, ...defaultPriority.filter((source) => !activeSources.has(source))];
    const mode = settings.policy.mode === "replace" ? "replace" : "missing";
    policyForm.querySelector<HTMLInputElement>(`input[name="metadata-mode"][value="${mode}"]`)!.checked = true;
    renderSourceOrder();
  }

  function profileLabel(definition: EmulatorDefinition, profile: EmulatorProfile): string {
    return `${definition.name} — ${profile.name || profile.platforms.join(", ")}`;
  }

  function renderEmulationProfiles(): void {
    const select = element<HTMLSelectElement>("emulation-profile");
    const previous = select.value;
    const options: HTMLOptionElement[] = [];
    for (const definition of emulationCatalog) {
      for (const profile of definition.profiles ?? []) {
        const available = Boolean(profile.executable || profile.flatpak_id);
        const option = new Option(`${profileLabel(definition, profile)}${available ? "" : " (não encontrado)"}`, profile.profile_id);
        option.disabled = !available;
        options.push(option);
      }
    }
    if (!options.length) {
      options.push(new Option("Nenhum perfil disponível", ""));
      options[0].disabled = true;
    }
    select.replaceChildren(...options);
    if ([...select.options].some((option) => option.value === previous && !option.disabled)) {
      select.value = previous;
    }
  }

  async function renderEmulation(): Promise<void> {
    const list = element("emulation-list");
    const statusLine = element("emulation-status");
    try {
      const [catalog, folders] = await Promise.all([
        Host.GetEmulators(),
        Host.GetEmulationFolderProfiles(),
      ]);
      emulationCatalog = (catalog ?? []) as EmulatorDefinition[];
      emulationFolders = folders ?? {};
      const cards = emulationCatalog.map((definition) => {
        const card = document.createElement("article");
        card.className = "emulation-card";
        const heading = document.createElement("div");
        heading.className = "emulation-card-head";
        const name = document.createElement("strong");
        name.textContent = definition.name;
        const state = document.createElement("span");
        state.className = `emulation-state ${definition.executable || definition.flatpak_id ? "is-detected" : "is-missing"}`;
        state.textContent = definition.executable || definition.flatpak_id ? "detectado" : "não encontrado";
        heading.append(name, state);
        if (definition.custom) {
          const badge = document.createElement("span");
          badge.className = "emulation-badge";
          badge.textContent = "manual";
          heading.append(badge);
          const remove = button("Remover", () => {
            if (!window.confirm(`Remover o emulador ${definition.name}?`)) return;
            void operation(async () => {
              await Host.RemoveEmulator(definition.id);
              await renderEmulation();
              await options.reload(false);
              statusLine.textContent = "Emulador removido.";
            });
          });
          remove.classList.add("danger-button");
          heading.append(remove);
        }
        card.append(heading);
        const profileList = document.createElement("p");
        profileList.className = "emulation-profiles";
        profileList.textContent = (definition.profiles ?? []).slice(0, 4)
          .map((profile) => `${profile.name} (${profile.extensions.join(", ")})`)
          .join("  ·  ");
        card.append(profileList);
        return card;
      });
      if (!cards.length) {
        const empty = document.createElement("p");
        empty.className = "properties-note";
        empty.textContent = "Nenhum emulador detectado. Adicione um manualmente abaixo.";
        cards.push(empty);
      }
      const entries = Object.entries(emulationFolders);
      if (entries.length) {
        const foldersTitle = document.createElement("h4");
        foldersTitle.textContent = "Pastas de ROMs configuradas";
        cards.push(foldersTitle);
        for (const [folder, profileID] of entries) {
          const row = document.createElement("div");
          row.className = "emulation-folder-row";
          const copy = document.createElement("span");
          const profile = emulationCatalog.flatMap((item) => item.profiles ?? []).find((item) => item.profile_id === profileID);
          copy.textContent = `${folder} — ${profile?.name ?? profileID}`;
          copy.title = folder;
          const remove = button("Remover", () => void operation(async () => {
            await Host.RemoveRomFolder(folder);
            await renderEmulation();
            await options.reload(false);
          }));
          remove.classList.add("danger-button");
          row.append(copy, remove);
          cards.push(row);
        }
      }
      list.replaceChildren(...cards);
      renderEmulationProfiles();
      statusLine.textContent = "";
    } catch (error) {
      statusLine.textContent = options.error(error);
    }
  }

  async function renderAppearance(): Promise<void> {
    const statusLine = element("appearance-status");
    try {
      const settings = await Host.GetUISettings();
      element<HTMLSelectElement>("appearance-theme").value = settings.theme;
      element<HTMLSelectElement>("appearance-view").value = settings.view_mode;
      element<HTMLInputElement>("appearance-transitions").checked = settings.transitions;
      element<HTMLInputElement>("appearance-gpu").checked = settings.gpu_acceleration;
      statusLine.textContent = "";
    } catch (error) {
      statusLine.textContent = options.error(error);
    }
  }

  const localImport = (id: string) => libraryPlugins[id]?.importsInstalled === true;

  async function operation(callback: () => Promise<void>, target = dialog): Promise<void> {
    if (busy) return;
    busy = true;
    const controls = Array.from(target.querySelectorAll<HTMLInputElement | HTMLSelectElement | HTMLButtonElement>(
      "button:not(.desktop-close), input, select",
    )).filter((control) => !control.disabled);
    controls.forEach((control) => { control.disabled = true; });
    try {
      await callback();
    } catch (error) {
      (target === metadataDialog ? metadataStatus : status).textContent = options.error(error);
    } finally {
      busy = false;
      controls.forEach((control) => { control.disabled = false; });
    }
  }

  function renderStores(): void {
    const stores = options.stores();
    const selected = stores.find((store) => store.id === activeStore) ?? stores[0];
    activeStore = selected?.id ?? "";
    storeList.replaceChildren(...stores.map((store) => {
      const item = button(store.name, () => {
        activeStore = store.id;
        showPanel("stores");
      });
      if (currentPanel === "stores" && store.id === activeStore) item.setAttribute("aria-current", "page");
      return item;
    }));
    storeName.textContent = selected?.name ?? "Bibliotecas";
    storeProperties.replaceChildren();
    storeActions.replaceChildren();
    if (!selected) {
      storeConfig.replaceChildren();
      delete storeConfig.dataset.plugin;
      return;
    }
    const plugin = libraryPlugins[selected.id];
    const local = plugin?.local;
    const properties = [
      ["Estado", local ? "Fonte local" : selected.connected ? "Conta configurada" : "Conta não configurada"],
      ["Origem", plugin?.source ?? "Biblioteca online"],
    ];
    if (selected.id === "steam") {
      properties.push(["Chave da Steam Web API", selected.api_key_saved ? "Cadastrada" : "Não cadastrada"]);
    }
    if (selected.clients?.length) {
      properties.push([
        selected.clients.length > 1 ? "Clientes possíveis" : "Cliente externo",
        selected.clients.map((client) => `${client.name} · ${client.installed ? "detectado" : "não detectado"}`).join(" / "),
      ]);
    }
    for (const [label, value] of properties) {
      const term = document.createElement("dt");
      const definition = document.createElement("dd");
      term.textContent = label;
      definition.textContent = value;
      storeProperties.append(term, definition);
    }
    if (selected.connected) storeActions.append(button("Atualizar biblioteca", () => queueLibrarySync([selected])));
    if (localImport(selected.id)) storeActions.append(button("Importar instalados", () => queueLibrarySync([selected], true)));
    if (storeConfig.dataset.plugin !== selected.id) {
      storeConfig.dataset.plugin = selected.id;
      storeConfig.innerHTML = plugin?.view ?? "";
      bindStoreSettings(selected);
    }
  }

  function bindStoreSettings(store: StoreInfo): void {
    storeConfig.querySelector("[data-store-login]")?.addEventListener("click", () => void operation(async () => {
      status.textContent = `Conectando a ${store.name}…`;
      const games = await Host.LoginStore(store.id);
      await options.reload(true);
      status.textContent = `${store.name} conectada. ${games?.length ?? 0} jogos recebidos.`;
    }));

    const account = storeConfig.querySelector<HTMLFormElement>("[data-steam-account]");
    const steamID = storeConfig.querySelector<HTMLInputElement>("#steam-id");
    const detectSteam = storeConfig.querySelector<HTMLButtonElement>("[data-steam-detect]");
    if (steamID && !steamID.value) {
      void Host.DetectSteamID().then((id) => {
        if (id && !steamID.value) steamID.value = id;
      }).catch(() => undefined);
    }
    detectSteam?.addEventListener("click", () => {
      detectSteam.disabled = true;
      void Host.DetectSteamID().then((id) => {
        if (steamID) steamID.value = id;
        status.textContent = `SteamID64 encontrado neste PC: ${id}`;
      }).catch((error) => {
        status.textContent = options.error(error);
      }).finally(() => {
        detectSteam.disabled = false;
      });
    });
    account?.addEventListener("submit", (event) => {
      event.preventDefault();
      const data = new FormData(account);
      void operation(async () => {
        const key = String(data.get("key") ?? "").trim();
        const clearKey = data.has("clearKey");
        await Host.ConfigureSteam(String(data.get("account") ?? ""), key, clearKey);
        account.reset();
        if (store.id === "steam" && !key && !clearKey) {
          options.report("SteamID salvo: instalados importados. Informe uma chave da Steam Web API para carregar os comprados não instalados.");
        }
        queueLibrarySync([{ ...store, connected: true }]);
      });
    });
    const folders = storeConfig.querySelector<HTMLButtonElement>("[data-plugin-folders]");
    folders?.addEventListener("click", () => {
      const kind = folders.dataset.pluginFolders === "rom" ? "rom" : "local";
      void options.configureFolders(kind).catch((error) => {
        status.textContent = options.error(error);
      });
    });
  }

  function queueLibrarySync(stores: StoreInfo[], installedOnly = false): void {
    const available = stores.filter((store) => store.connected || localImport(store.id));
    if (!available.length) {
      status.textContent = "Nenhuma biblioteca disponível para atualizar.";
      return;
    }
    dialog.close();
    downloads.enqueue(
      "Bibliotecas",
      available,
      async (store) => {
        try {
          if (installedOnly || !store.connected) {
            await Host.ImportInstalledGames(store.id);
          } else {
            await Host.SyncStore(store.id);
          }
        } catch (error) {
          throw new Error(`${store.name}: ${options.error(error)}`, { cause: error });
        }
      },
      () => options.reload(true),
    );
  }

  function open(panel: Panel, storeID?: string, message = ""): void {
    if (busy) return;
    if (panel === "plugins") {
      panel = (Object.keys(metadataPlugins)[0] as Panel) ?? "library";
    }
    if (storeID) activeStore = storeID;
    showPanel(panel);
    status.textContent = message;
    if (!dialog.open) dialog.showModal();
    void refreshProviders();
  }

  async function refreshProviders(): Promise<void> {
    try {
      settings = await Host.GetMetadataSettings();
      updateProviderStates();
    } catch (error) {
      status.textContent = options.error(error);
    }
  }

  function openMetadata(game = options.selected()): void {
    if (busy || metadataDialog.open) return;
    metadataGame = game;
    const selection = policyForm.querySelector<HTMLInputElement>('[value="selected"]')!;
    selection.disabled = !game;
    selection.checked = !!game;
    if (!game) policyForm.querySelector<HTMLInputElement>('[value="all"]')!.checked = true;
    sourceList.replaceChildren();
    metadataStatus.textContent = "Carregando configurações…";
    metadataDialog.showModal();
    void operation(async () => {
      settings = await Host.GetMetadataSettings();
      renderPolicy();
      metadataStatus.textContent = "";
    }, metadataDialog);
  }

  async function savePolicy(): Promise<MetadataSettings["policy"]> {
    const sources = sourceOrder.filter((source) => activeSources.has(source));
    const mode = policyForm.querySelector<HTMLInputElement>('input[name="metadata-mode"]:checked')?.value ?? "missing";
    await Host.SaveMetadataPolicy(automatic.checked, sources, mode);
    return { automatic: automatic.checked, sources, mode };
  }

  function enqueueMetadata(ids: string[], policy: MetadataSettings["policy"]): void {
    downloads.enqueue(
      "Metadados",
      [...new Set(ids)],
      (id) => Host.DownloadGameMetadata(id, policy.sources, policy.mode),
      (games) => options.applyDownloads(games.filter((game): game is Game => game !== null)),
    );
  }

  dialog.querySelectorAll<HTMLButtonElement>("[data-integration-panel-link]").forEach((link) => {
    link.addEventListener("click", () => showPanel((link.dataset.integrationPanelLink || "library") as Panel));
  });
  element("integration-metadata-settings").addEventListener("click", () => {
    dialog.close();
    openMetadata();
  });
  element("library-save").addEventListener("click", () => void saveLibrarySettings());
  element("library-add-game").addEventListener("click", () => {
    dialog.close();
    options.addGame();
  });
  element("library-clear").addEventListener("click", () => void clearLibrary());

  element<HTMLButtonElement>("emulation-scan").addEventListener("click", () => void operation(async () => {
    const statusLine = element("emulation-status");
    const folder = element<HTMLInputElement>("emulation-folder").value.trim();
    const profileID = element<HTMLSelectElement>("emulation-profile").value;
    if (!folder) {
      statusLine.textContent = "Informe a pasta de ROMs.";
      return;
    }
    if (!profileID) {
      statusLine.textContent = "Escolha um perfil de emulador.";
      return;
    }
    statusLine.textContent = "Escaneando ROMs…";
    const games = await Host.AddEmulationFolder(folder, profileID);
    await renderEmulation();
    await options.reload(false);
    statusLine.textContent = `${games?.length ?? 0} ROMs importadas desta pasta.`;
  }));

  element<HTMLFormElement>("emulator-form").addEventListener("submit", (event) => {
    event.preventDefault();
    void operation(async () => {
      const statusLine = element("emulator-status");
      const name = element<HTMLInputElement>("emulator-name").value.trim();
      const executable = element<HTMLInputElement>("emulator-executable").value.trim();
      const flatpakID = element<HTMLInputElement>("emulator-flatpak").value.trim();
      const profileName = element<HTMLInputElement>("emulator-profile-name").value.trim();
      const platform = element<HTMLInputElement>("emulator-platform").value.trim();
      const extensions = element<HTMLInputElement>("emulator-extensions").value.split(/[\s,;]+/).filter(Boolean);
      const argsTemplate = element<HTMLInputElement>("emulator-args").value.trim() || `"{ImagePath}"`;
      if (!name || !profileName || !platform || !extensions.length) {
        statusLine.textContent = "Preencha nome, perfil, plataforma e extensões.";
        return;
      }
      if (!executable && !flatpakID) {
        statusLine.textContent = "Informe o executável ou o Flatpak ID.";
        return;
      }
      const id = name.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "")
        .replace(/[^a-z0-9._-]+/g, "-").replace(/^-+|-+$/g, "") || "custom-emulator";
      await Host.SaveEmulator({
        id,
        name,
        executable,
        flatpak_id: flatpakID,
        profiles: [{
          profile_id: `${id}:default`,
          emulator_id: id,
          name: profileName,
          platforms: [platform],
          extensions,
          executable,
          flatpak_id: flatpakID,
          args_template: argsTemplate,
          folder_aliases: [],
          priority: 100,
          builtin: false,
        }],
      });
      element<HTMLFormElement>("emulator-form").reset();
      element<HTMLInputElement>("emulator-args").value = `"{ImagePath}"`;
      await renderEmulation();
      statusLine.textContent = "Emulador salvo. Escolha um perfil e escaneie uma pasta.";
    });
  });

  element<HTMLButtonElement>("appearance-save").addEventListener("click", () => void operation(async () => {
    const statusLine = element("appearance-status");
    const settings: UISettingsLike = {
      theme: element<HTMLSelectElement>("appearance-theme").value,
      view_mode: element<HTMLSelectElement>("appearance-view").value,
      transitions: element<HTMLInputElement>("appearance-transitions").checked,
      gpu_acceleration: element<HTMLInputElement>("appearance-gpu").checked,
    };
    statusLine.textContent = "Salvando…";
    await Host.SaveUISettings(settings);
    options.applyAppearance(applyUISettings(settings));
    options.setViewMode(settings.view_mode === "grid" ? "grid" : "details");
    statusLine.textContent = settings.gpu_acceleration
      ? "Aparência salva. A aceleração de GPU já está ativa."
      : "Aparência salva. A GPU será desativada ao reiniciar o Playnux.";
  }));

  element("integrations-close").addEventListener("click", () => dialog.close());
  element("integrations-done").addEventListener("click", () => dialog.close());
  element("metadata-close").addEventListener("click", () => metadataDialog.close());
  element("metadata-done").addEventListener("click", () => metadataDialog.close());
  element("sync-libraries").addEventListener("click", () => queueLibrarySync(options.stores()));
  element("metadata-save").addEventListener("click", () => void operation(async () => {
    await savePolicy();
    metadataStatus.textContent = "Padrão de metadados salvo.";
  }, metadataDialog));
  policyForm.addEventListener("submit", (event) => {
    event.preventDefault();
    const scope = policyForm.querySelector<HTMLInputElement>('input[name="scope"]:checked')?.value;
    const games = scope === "selected" ? (metadataGame ? [metadataGame] : [])
      : scope === "filtered" ? options.filteredGames() : options.games();
    if (!games.length) {
      metadataStatus.textContent = "Nenhum jogo selecionado.";
      return;
    }
    void operation(async () => {
      const policy = await savePolicy();
      metadataDialog.close();
      enqueueMetadata(games.map((game) => game.id), policy);
    }, metadataDialog);
  });

  for (const help of dialog.querySelectorAll<HTMLButtonElement>("[data-help-url]")) {
    help.addEventListener("click", () => {
      const url = help.dataset.helpUrl;
      if (url) void Browser.OpenURL(url).catch((error) => {
        status.textContent = options.error(error);
      });
    });
  }

  for (const form of dialog.querySelectorAll<HTMLFormElement>("[data-provider]")) {
    const update = (clear: boolean) => {
      const data = new FormData(form);
      void operation(async () => {
        await Host.SetMetadataCredentials(form.dataset.provider ?? "", clear ? "" : String(data.get("client") ?? ""), clear ? "" : String(data.get("key") ?? ""));
        form.reset();
        settings = await Host.GetMetadataSettings();
        updateProviderStates();
        status.textContent = clear ? "Credenciais salvas removidas." : "Credenciais salvas.";
      });
    };
    form.addEventListener("submit", (event) => { event.preventDefault(); update(false); });
    form.querySelector("[data-clear-provider]")?.addEventListener("click", () => update(true));
  }

  return {
    open,
    openMetadata,
    refreshStores: () => { if (dialog.open) renderStores(); },
    downloadNew(ids: string[]) {
      if (!ids.length) return;
      void Host.GetMetadataSettings().then(({ policy }) => {
        if (policy.automatic) enqueueMetadata(ids, policy);
      }).catch((error) => options.report(options.error(error), true));
    },
  };
}
