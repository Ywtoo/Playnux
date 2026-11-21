import { Browser } from "@wailsio/runtime";
import * as Host from "../bindings/github.com/playnux/ui/host/host.js";
import type { Game } from "../bindings/github.com/playnux/core/models/models.js";
import type { WebImage } from "../bindings/github.com/playnux/core/metadata/models.js";
import type { ArtworkField } from "./game-information";
import { sourceLabel } from "./format";

export type PropertiesTab = "general" | "artwork" | "launch";

interface Options {
  error: (error: unknown) => string;
  onGameUpdated: (game: Game) => void;
  toast: (message: string, isError?: boolean) => void;
}

interface RunnerSelection {
  runner: "auto" | "wine" | "proton";
  protonPath: string;
}

function element<T extends HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (!node) throw new Error(`Elemento ausente: ${id}`);
  return node as T;
}

function lines(value: string): string[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function dateValue(value: string | null | undefined): string {
  if (!value) return "";
  const match = /^\d{4}-\d{2}-\d{2}/.exec(value);
  return match ? match[0] : "";
}

function manualWindowsPath(game: Game): string {
  const path = game.game_id.startsWith("manual:")
    ? game.game_id.slice("manual:".length)
    : game.executable || "";
  return path.toLowerCase().endsWith(".exe") ? path : "";
}

export function setupGameProperties(options: Options) {
  const dialog = element<HTMLDialogElement>("game-properties");
  const close = element<HTMLButtonElement>("properties-close");
  const cover = element<HTMLImageElement>("properties-cover");
  const gameName = element<HTMLElement>("properties-game-name");
  const gameMeta = element<HTMLElement>("properties-game-meta");
  const runtimeBadge = element<HTMLElement>("properties-runtime-badge");
  const tabs = Array.from(dialog.querySelectorAll<HTMLButtonElement>("[data-properties-tab]"));
  const panels = Array.from(dialog.querySelectorAll<HTMLElement>("[data-properties-panel]"));
  let game: Game | null = null;
  let busy = false;

  function showTab(target: PropertiesTab): void {
    for (const button of tabs) {
      const active = button.dataset.propertiesTab === target;
      button.setAttribute("aria-current", active ? "page" : "false");
    }
    for (const panel of panels) {
      panel.hidden = panel.dataset.propertiesPanel !== target;
    }
    if (target === "artwork") refreshCurrentArtwork();
  }

  // ── Cabeçalho ────────────────────────────────────────────────────────────
  function refreshHeader(): void {
    if (!game) return;
    const target = game;
    const image = game.cover_image || game.background_image || "";
    cover.hidden = !image;
    if (image) cover.src = image;
    else cover.removeAttribute("src");
    gameName.textContent = game.name;
    gameMeta.textContent = [sourceLabel(game), game.is_installed ? "Instalado" : "Não instalado"]
      .filter(Boolean)
      .join(" · ");
    runtimeBadge.hidden = true;
    void Host.GetGameRuntime(game.id).then((runtime) => {
      if (game !== target || !dialog.open) return;
      const value = runtime.running || runtime.configured;
      runtimeBadge.hidden = !value;
      runtimeBadge.textContent = runtime.running ? `Em execução · ${runtime.running}` : `Runner · ${value}`;
    }).catch(() => undefined);
  }

  // ── Geral ────────────────────────────────────────────────────────────────
  const generalForm = element<HTMLFormElement>("general-form");
  const name = element<HTMLInputElement>("general-name");
  const release = element<HTMLInputElement>("general-release");
  const version = element<HTMLInputElement>("general-version");
  const critic = element<HTMLInputElement>("general-critic");
  const community = element<HTMLInputElement>("general-community");
  const userScore = element<HTMLInputElement>("general-user");
  const trailer = element<HTMLInputElement>("general-trailer");
  const description = element<HTMLTextAreaElement>("general-description");
  const genres = element<HTMLInputElement>("general-genres");
  const developers = element<HTMLInputElement>("general-developers");
  const publishers = element<HTMLInputElement>("general-publishers");
  const platforms = element<HTMLInputElement>("general-platforms");
  const tags = element<HTMLInputElement>("general-tags");
  const categories = element<HTMLInputElement>("general-categories");
  const features = element<HTMLInputElement>("general-features");
  const metadataMode = element<HTMLSelectElement>("general-metadata-mode");
  const metadataFetch = element<HTMLButtonElement>("general-fetch");
  const generalStatus = element<HTMLElement>("general-status");

  function fillGeneral(): void {
    if (!game) return;
    name.value = game.name;
    release.value = dateValue(game.release_date);
    version.value = game.version || "";
    critic.value = game.critic_score == null ? "" : String(game.critic_score);
    community.value = game.community_score == null ? "" : String(game.community_score);
    userScore.value = game.user_score == null ? "" : String(game.user_score);
    trailer.value = game.trailer_url || "";
    description.value = game.description || "";
    genres.value = (game.genres || []).join(", ");
    developers.value = (game.developers || []).join(", ");
    publishers.value = (game.publishers || []).join(", ");
    platforms.value = (game.platforms || []).join(", ");
    tags.value = (game.tags || []).join(", ");
    categories.value = (game.categories || []).join(", ");
    features.value = (game.features || []).join(", ");
    generalStatus.textContent = "";
  }

  function scoreValue(field: HTMLInputElement): number | null {
    if (field.value.trim() === "") return null;
    const value = Number(field.value);
    return Number.isFinite(value) ? value : null;
  }

  function applyGame(updated: Game): void {
    game = updated;
    options.onGameUpdated(updated);
    refreshHeader();
  }

  generalForm.addEventListener("submit", (event) => {
    event.preventDefault();
    if (!game || busy) return;
    const target = game;
    busy = true;
    generalStatus.textContent = "Salvando…";
    void Host.SaveGameDetails(target.id, {
      name: name.value,
      description: description.value,
      release_date: release.value,
      version: version.value,
      genres: lines(genres.value),
      developers: lines(developers.value),
      publishers: lines(publishers.value),
      platforms: lines(platforms.value),
      tags: lines(tags.value),
      categories: lines(categories.value),
      features: lines(features.value),
      critic_score: scoreValue(critic),
      community_score: scoreValue(community),
      user_score: scoreValue(userScore),
      trailer_url: trailer.value,
    }).then((updated) => {
      if (!updated || game !== target) return;
      applyGame(updated);
      fillGeneral();
      generalStatus.textContent = "Metadados salvos.";
      options.toast("Propriedades atualizadas.");
    }).catch((error) => {
      generalStatus.textContent = options.error(error);
    }).finally(() => {
      busy = false;
    });
  });

  metadataFetch.addEventListener("click", () => {
    if (!game || busy) return;
    const target = game;
    busy = true;
    metadataFetch.disabled = true;
    generalStatus.textContent = "Buscando metadados…";
    void Host.GetMetadataSettings().then(async (settings) => {
      const updated = await Host.DownloadGameMetadata(target.id, settings.policy.sources, metadataMode.value);
      if (game !== target || !updated) return;
      applyGame(updated);
      fillGeneral();
      generalStatus.textContent = "Metadados atualizados pelos provedores.";
    }).catch((error) => {
      generalStatus.textContent = options.error(error);
    }).finally(() => {
      busy = false;
      metadataFetch.disabled = false;
    });
  });

  // ── Imagens ──────────────────────────────────────────────────────────────
  const artworkForm = element<HTMLFormElement>("artwork-form");
  const variantField = element<HTMLElement>("artwork-variant-field");
  const variantSelect = element<HTMLSelectElement>("artwork-variant");
  const variantButton = element<HTMLButtonElement>("artwork-use-variant");
  const sourceSelect = element<HTMLSelectElement>("artwork-source");
  const searchTitle = element("artwork-search-title");
  const providerButton = element<HTMLButtonElement>("artwork-steamgriddb");
  const browserButton = element<HTMLButtonElement>("artwork-browser");
  const artworkSlots = dialog.querySelectorAll<HTMLButtonElement>("[data-artwork-field]");
  const currentArtwork = dialog.querySelectorAll<HTMLImageElement>("[data-current-art]");
  const fieldLabels: Record<ArtworkField, string> = { cover: "capa", background: "banner", logo: "logotipo", icon: "ícone" };
  let artworkField: ArtworkField = "cover";
  const artworkFile = element<HTMLInputElement>("artwork-file");
  const preview = element<HTMLImageElement>("artwork-preview");
  const artworkStatus = element<HTMLElement>("artwork-status");
  const queryInput = element<HTMLInputElement>("artwork-query");
  const searchButton = element<HTMLButtonElement>("artwork-search");
  const imageResults = element("artwork-results");
  let selectedImage: { image: WebImage; provider: string } | null = null;
  let imageSearch = 0;
  let previewURL = "";

  function fillVariants(): void {
    const variants = game?.variants ?? [];
    const unified = variants.length > 1;
    variantField.hidden = !unified;
    if (!unified) {
      variantSelect.replaceChildren();
      return;
    }
    variantSelect.replaceChildren(...variants.map((variant) => {
      const label = `${variant.source || variant.plugin_id}${variant.is_installed ? " · instalado" : ""}`;
      return new Option(label, variant.id);
    }));
    variantSelect.value = game?.id ?? variants[0].id;
  }

  variantButton.addEventListener("click", () => {
    if (!game || busy) return;
    const target = game;
    const sourceID = variantSelect.value;
    if (!sourceID) return;
    busy = true;
    variantButton.disabled = true;
    artworkStatus.textContent = "Aplicando imagens da origem escolhida…";
    void Host.CopyGameArtwork(target.id, sourceID).then((updated) => {
      if (!updated || game !== target) return;
      applyGame(updated);
      fillVariants();
      refreshCurrentArtwork();
      artworkStatus.textContent = "Imagens da origem aplicadas.";
    }).catch((error) => {
      artworkStatus.textContent = options.error(error);
    }).finally(() => {
      busy = false;
      variantButton.disabled = false;
    });
  });

  function refreshCurrentArtwork(): void {
    if (!game) return;
    const values: Record<ArtworkField, string | undefined> = {
      cover: game.cover_image, background: game.background_image, logo: game.logo_image, icon: game.icon,
    };
    currentArtwork.forEach((image) => {
      const path = values[image.dataset.currentArt as ArtworkField];
      image.onerror = () => { image.hidden = true; };
      image.hidden = !path;
      if (path) image.src = path;
      else image.removeAttribute("src");
    });
  }

  function resetSearch(): void {
    imageSearch++;
    selectedImage = null;
    imageResults.replaceChildren();
    searchButton.disabled = false;
    const label = fieldLabels[artworkField];
    const google = sourceSelect.value === "google";
    searchTitle.textContent = `Buscar ${label}`;
    queryInput.value = game ? (google ? `${game.name} ${label}` : game.name) : "";
    browserButton.hidden = !google;
    providerButton.hidden = google;
    artworkSlots.forEach((slot) => slot.setAttribute("aria-pressed", String(slot.dataset.artworkField === artworkField)));
    artworkStatus.textContent = "";
  }

  function refreshPreview(): void {
    if (previewURL) URL.revokeObjectURL(previewURL);
    const file = artworkFile.files?.[0];
    previewURL = file ? URL.createObjectURL(file) : "";
    const src = previewURL || selectedImage?.image.thumbnail || "";
    preview.hidden = !src;
    if (src) preview.src = src;
    else preview.removeAttribute("src");
  }

  artworkSlots.forEach((slot) => slot.addEventListener("click", () => {
    artworkField = slot.dataset.artworkField as ArtworkField;
    artworkFile.value = "";
    resetSearch();
    refreshPreview();
    queryInput.focus();
  }));
  sourceSelect.addEventListener("change", () => {
    resetSearch();
    refreshPreview();
  });
  artworkFile.addEventListener("change", () => {
    selectedImage = null;
    for (const control of imageResults.querySelectorAll("button")) control.setAttribute("aria-pressed", "false");
    refreshPreview();
  });

  async function searchImages(): Promise<void> {
    const query = queryInput.value.trim();
    if (!game || !query || searchButton.disabled) return;
    const request = ++imageSearch;
    selectedImage = null;
    imageResults.replaceChildren();
    refreshPreview();
    searchButton.disabled = true;
    artworkStatus.textContent = "Buscando imagens…";
    try {
      const provider = sourceSelect.value;
      const results = (provider === "google"
        ? await Host.SearchGoogleImages(query, artworkField === "logo")
        : await Host.SearchProviderArtwork(query, artworkField)) ?? [];
      if (request !== imageSearch) return;
      for (const result of results) {
        const thumbnail = document.createElement("img");
        thumbnail.src = result.thumbnail;
        thumbnail.alt = "";
        thumbnail.loading = "lazy";
        thumbnail.referrerPolicy = "no-referrer";
        const size = result.width && result.height ? `${result.width} × ${result.height}` : "";
        const control = document.createElement("button");
        control.type = "button";
        control.className = "ghost-button";
        control.setAttribute("aria-label", size ? `Selecionar imagem ${size}` : "Selecionar imagem");
        control.setAttribute("aria-pressed", "false");
        control.textContent = size;
        control.prepend(thumbnail);
        control.addEventListener("click", () => {
          selectedImage = { image: result, provider };
          artworkFile.value = "";
          for (const other of imageResults.querySelectorAll("button")) {
            other.setAttribute("aria-pressed", String(control === other));
          }
          refreshPreview();
          artworkStatus.textContent = "Imagem selecionada. Clique em Aplicar imagem para salvar.";
        });
        imageResults.append(control);
      }
      artworkStatus.textContent = results.length ? `${results.length} imagens encontradas.` : "Nenhuma imagem encontrada.";
    } catch (error) {
      if (request === imageSearch) artworkStatus.textContent = options.error(error);
    } finally {
      if (request === imageSearch) searchButton.disabled = false;
    }
  }

  searchButton.addEventListener("click", () => void searchImages());
  queryInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      void searchImages();
    }
  });
  browserButton.addEventListener("click", () => {
    const query = queryInput.value.trim();
    if (!query) return;
    const url = `https://www.google.com/search?tbm=isch&q=${encodeURIComponent(query)}`;
    void Browser.OpenURL(url).catch((error) => {
      artworkStatus.textContent = options.error(error);
    });
  });
  providerButton.addEventListener("click", () => {
    if (!game || busy) return;
    const target = game;
    const field = artworkField;
    busy = true;
    artworkStatus.textContent = "Buscando a melhor imagem…";
    void Host.ApplyProviderArtwork(target.id, field).then(() => Host.GetGame(target.id)).then((updated) => {
      if (!updated || game !== target) return;
      applyGame(updated);
      refreshCurrentArtwork();
      artworkStatus.textContent = "Imagem aplicada.";
    }).catch((error) => {
      artworkStatus.textContent = options.error(error);
    }).finally(() => {
      busy = false;
    });
  });
  artworkForm.addEventListener("submit", (event) => {
    event.preventDefault();
    const file = artworkFile.files?.[0];
    const target = game;
    const remote = selectedImage;
    if (!target || (!file && !remote)) {
      artworkStatus.textContent = "Selecione uma miniatura ou um arquivo.";
      return;
    }
    if (file && file.size > 3 * 1024 * 1024) {
      artworkStatus.textContent = "Selecione uma imagem de até 3 MB.";
      return;
    }
    const field = artworkField;
    busy = true;
    artworkStatus.textContent = "Aplicando imagem…";
    void (async () => {
      if (remote) {
        if (remote.provider === "google") await Host.ApplyWebImage(target.id, field, remote.image);
        else await Host.ApplySteamGridImage(target.id, field, remote.image);
      } else if (file) {
        const encoded = await new Promise<string>((resolve, reject) => {
          const reader = new FileReader();
          reader.onload = () => resolve(String(reader.result).split(",", 2)[1] ?? "");
          reader.onerror = () => reject(reader.error);
          reader.readAsDataURL(file);
        });
        await Host.SaveGameArtwork(target.id, field, encoded);
      }
      const updated = await Host.GetGame(target.id);
      if (!updated || game !== target) return;
      applyGame(updated);
      refreshCurrentArtwork();
      artworkFile.value = "";
      selectedImage = null;
      resetSearch();
      refreshPreview();
      artworkStatus.textContent = "Imagem aplicada.";
    })().catch((error) => {
      artworkStatus.textContent = options.error(error);
    }).finally(() => {
      busy = false;
    });
  });

  // ── Inicialização ────────────────────────────────────────────────────────
  const launchForm = element<HTMLFormElement>("launch-form");
  const localSettings = element<HTMLFieldSetElement>("launch-local-settings");
  const runnerSelect = element<HTMLSelectElement>("launch-runner");
  const directory = element<HTMLInputElement>("launch-directory");
  const argumentsField = element<HTMLTextAreaElement>("launch-arguments");
  const commandPreview = element<HTMLElement>("launch-command");
  const runnerHint = element<HTMLElement>("launch-runner-hint");
  const launchProvider = element<HTMLElement>("launch-provider");
  const runtime = element<HTMLElement>("launch-runtime");
  const launchStatus = element<HTMLElement>("launch-status");
  const proton = element<HTMLButtonElement>("launch-protondb");
  const save = element<HTMLButtonElement>("launch-save");
  let runners: { name: string; path: string; kind: string; recommended: boolean }[] = [];
  let windowsExecutable = "";

  function selectedRunner(): RunnerSelection {
    const value = runnerSelect.value;
    if (value === "auto" || value === "wine") return { runner: value, protonPath: "" };
    return { runner: "proton", protonPath: value };
  }

  function updateCommandPreview(): void {
    if (!game) return;
    const choice = selectedRunner();
    const extra = argumentsField.value.split("\n").map((arg) => arg.trim()).filter(Boolean);
    const suffix = extra.length ? " " + extra.map((arg) => JSON.stringify(arg)).join(" ") : "";
    const exe = windowsExecutable;
    if (exe) {
      // Store games default to their native client; manual games default to Proton.
      const isManual = game.plugin_id === "linux";
      const protonPath = choice.runner === "proton"
        ? choice.protonPath
        : choice.runner === "auto" && isManual
          ? runners.find((item) => item.kind === "proton")?.path || ""
          : "";
      if (protonPath) {
        commandPreview.textContent = `${JSON.stringify(protonPath)} run ${JSON.stringify(exe)}${suffix}`;
        runnerHint.textContent = "O Proton usa um prefixo isolado do Playnux. A Steam e o Heroic não são alterados. O comando exato fica visível acima.";
        return;
      }
      if (choice.runner === "wine") {
        commandPreview.textContent = `wine ${JSON.stringify(exe)}${suffix}`;
        runnerHint.textContent = "O Wine do sistema será usado diretamente, sem alterar o cliente da loja.";
        return;
      }
      commandPreview.textContent = game.plugin_id === "linux"
        ? `${game.executable || "wine"} ${JSON.stringify(exe)}${suffix}`
        : `cliente nativo (${sourceLabel(game)}) — selecione um Proton para iniciar com Playnux`;
      runnerHint.textContent = game.plugin_id === "linux"
        ? "Nenhum Proton foi encontrado; o jogo .exe será iniciado com Wine do sistema."
        : "Por padrão o jogo abre pelo cliente da loja. Escolha um Proton para usar o prefixo isolado do Playnux.";
      return;
    }
    commandPreview.textContent = `${game.executable || "—"}${suffix}`;
    runnerHint.textContent = game.plugin_id === "emulation"
      ? "Os argumentos acima são passados ao emulador escolhido para esta ROM."
      : "Este jogo é nativo do Linux; o seletor Proton se aplica apenas a executáveis .exe.";
  }

  function fillRunners(): void {
    const previous = savedRunner;
    runnerSelect.replaceChildren();
    const auto = document.createElement("option");
    auto.value = "auto";
    auto.textContent = "Automático (Proton se disponível)";
    runnerSelect.append(auto);
    const wine = document.createElement("option");
    wine.value = "wine";
    wine.textContent = "Wine do sistema";
    runnerSelect.append(wine);
    for (const runner of runners.filter((item) => item.kind === "proton")) {
      const option = document.createElement("option");
      option.value = runner.path;
      option.textContent = runner.recommended ? `${runner.name} (recomendado)` : runner.name;
      runnerSelect.append(option);
    }
    runnerSelect.value = previous && [...runnerSelect.options].some((option) => option.value === previous) ? previous : "auto";
  }

  let savedRunner = "auto";

  function fillLaunch(): void {
    if (!game) return;
    const target = game;
    const native = game.plugin_id === "linux" || game.plugin_id === "emulation";
    windowsExecutable = manualWindowsPath(game);
    launchStatus.textContent = "";
    runtime.replaceChildren();
    runtime.hidden = true;
    proton.hidden = !(game.plugin_id !== "emulation" && game.is_installed);

    void Host.GetGameRuntime(game.id).then((result) => {
      if (game !== target || !dialog.open) return;
      const rows = [["Runner configurado", result.configured], ["Runner em uso", result.running]].filter(([, value]) => value);
      for (const [label, value] of rows) {
        const term = document.createElement("dt");
        const definition = document.createElement("dd");
        term.textContent = label;
        definition.textContent = value;
        runtime.append(term, definition);
      }
      runtime.hidden = rows.length === 0;
      const badge = result.running || result.configured;
      runtimeBadge.hidden = !badge;
      runtimeBadge.textContent = result.running ? `Em execução · ${result.running}` : `Runner · ${badge}`;
    }).catch(() => undefined);

    // The Core resolves the Windows executable for store games too. This is
    // what allows choosing an isolated Proton without touching Steam/Heroic.
    const resolveExecutable = native
      ? Promise.resolve(windowsExecutable)
      : Host.GetWindowsExecutable(game.id).catch(() => "");

    void resolveExecutable.then((exe) => {
      if (game !== target || !dialog.open) return;
      if (exe) windowsExecutable = exe;
      const local = native || Boolean(windowsExecutable);
      localSettings.disabled = !local;
      runnerSelect.disabled = !windowsExecutable;
      save.hidden = !local;
      launchProvider.textContent = windowsExecutable
        ? "Este jogo Windows pode iniciar pelo cliente da loja ou por um Proton/Wine isolado do Playnux, sem alterar configurações da Steam ou do Heroic."
        : native
          ? "O Playnux inicia este jogo diretamente. Ajuste abaixo os argumentos e o motor de compatibilidade."
          : game.plugin_id === "steam"
            ? "Abra o jogo pela Steam ou instale-o no disco para habilitar um Proton isolado no Playnux."
            : game.plugin_id === "gog" || game.plugin_id === "epic"
              ? "Instale o jogo pelo Heroic para habilitar um Proton isolado ou ajuste Wine/Proton no cliente."
              : `A inicialização deste jogo é gerenciada pela origem ${sourceLabel(game)}.`;
      if (!local) {
        updateCommandPreview();
        return;
      }
      // Load runners before restoring the saved choice: otherwise the <select>
      // stays on "auto" and looks like the Proton option was not saved.
      const loadRunners = windowsExecutable
        ? Host.GetCompatibilityRunners().then((list) => { runners = list ?? []; fillRunners(); })
        : Promise.resolve();
      void loadRunners.then(() => Host.GetLaunchOptions(target.id)).then((settings) => {
        if (game !== target || !dialog.open) return;
        directory.value = settings.directory || "";
        argumentsField.value = (settings.arguments ?? []).join("\n");
        const choice = settings.runner === "proton" ? settings.proton_path || "auto" : settings.runner === "wine" ? "wine" : "auto";
        savedRunner = choice;
        if ([...runnerSelect.options].some((option) => option.value === choice)) runnerSelect.value = choice;
        if (!settings.runner) runnerSelect.value = "auto";
        updateCommandPreview();
      });
    }).catch((error) => {
      launchStatus.textContent = options.error(error);
    });
  }

  runnerSelect.addEventListener("change", () => {
    savedRunner = runnerSelect.value;
    updateCommandPreview();
  });
  argumentsField.addEventListener("input", updateCommandPreview);
  launchForm.addEventListener("submit", (event) => {
    event.preventDefault();
    if (!game || busy || launchForm.querySelector("fieldset")?.disabled) return;
    const target = game;
    const choice = selectedRunner();
    const args = argumentsField.value.split("\n").map((arg) => arg.trim()).filter(Boolean);
    busy = true;
    save.disabled = true;
    launchStatus.textContent = "Salvando…";
    void Host.SaveLaunchOptions(target.id, directory.value, args, choice.runner, choice.protonPath).then(() => {
      if (game !== target) return;
      launchStatus.textContent = "Inicialização salva.";
      options.toast("Inicialização atualizada.");
      fillLaunch();
    }).catch((error) => {
      launchStatus.textContent = options.error(error);
    }).finally(() => {
      busy = false;
      save.disabled = false;
    });
  });
  proton.addEventListener("click", () => {
    if (!game) return;
    const target = game;
    import("./plugins/protondb").then(({ openProtonReports }) => openProtonReports(target)).catch((error) => {
      launchStatus.textContent = options.error(error);
    });
  });

  // ── Ciclo de vida ────────────────────────────────────────────────────────
  close.addEventListener("click", () => dialog.close());
  dialog.addEventListener("close", () => {
    game = null;
    busy = false;
    artworkForm.reset();
    launchForm.reset();
    imageResults.replaceChildren();
    preview.hidden = true;
    preview.removeAttribute("src");
  });
  for (const button of tabs) {
    button.addEventListener("click", () => showTab(button.dataset.propertiesTab as PropertiesTab));
  }

  return {
    open(target: Game, initial: PropertiesTab = "general"): void {
      if (busy) return;
      game = target;
      fillGeneral();
      fillLaunch();
      fillVariants();
      refreshCurrentArtwork();
      resetSearch();
      refreshPreview();
      showTab(initial);
      refreshHeader();
      if (!dialog.open) dialog.showModal();
    },
    refresh(): void {
      if (dialog.open && game) {
        refreshHeader();
        fillGeneral();
        refreshCurrentArtwork();
      }
    },
  };
}
