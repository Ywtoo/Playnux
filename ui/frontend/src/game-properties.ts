import { language, t } from "./i18n";
import { Browser } from "@wailsio/runtime";
import * as Host from "../bindings/github.com/playnux/ui/host/host.js";
import type { Game } from "../bindings/github.com/playnux/core/models/models.js";
import type { WebImage } from "../bindings/github.com/playnux/core/metadata/models.js";
import type { ArtworkField } from "./game-information";
import { sourceLabel } from "./format";
import { hasProtonReports, officialProtonDownloads, openProtonReports, protonDownload, recommendationText, runnerMatches, runtimeLabel, type ProtonIntelligence } from "./plugins/protondb";
import type { Runner } from "../bindings/github.com/playnux/core/runner/wine/models.js";

export type PropertiesTab = "general" | "artwork" | "launch";

interface Options {
  error: (error: unknown) => string;
  recordGameError: (game: string, action: string, message: string) => void;
  onGameUpdated: (game: Game) => void;
  toast: (message: string, isError?: boolean) => void;
}

interface RunnerSelection {
  runner: "auto" | "wine" | "proton";
  protonPath: string;
}

function element<T extends HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (!node) throw new Error(t("Missing element {0}", id));
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
    const image = game.cover_image || game.background_image || "";
    cover.hidden = !image;
    if (image) cover.src = image;
    else cover.removeAttribute("src");
    gameName.textContent = game.name;
    gameMeta.textContent = [sourceLabel(game), game.is_installed ? t("Installed") : t("Not installed")]
      .filter(Boolean)
      .join(" · ");

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
    generalStatus.textContent = t("Saving…");
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
      generalStatus.textContent = t("Metadata saved.");
      options.toast(t("Properties updated."));
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
    generalStatus.textContent = t("Fetching metadata…");
    void Host.GetMetadataSettings().then(async (settings) => {
      const updated = await Host.DownloadGameMetadata(target.id, settings.policy.sources, metadataMode.value);
      if (game !== target || !updated) return;
      applyGame(updated);
      fillGeneral();
      generalStatus.textContent = t("Metadata updated from providers.");
    }).catch((error) => {
      const message = options.error(error);
      generalStatus.textContent = message;
      options.recordGameError(target.name, t("Metadata"), message);
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
  const fieldLabels: Record<ArtworkField, string> = { cover: t("cover"), background: "banner", logo: t("logo"), icon: t("icon") };
  let artworkField: ArtworkField = "cover";
  const artworkFile = element<HTMLInputElement>("artwork-file");
  const artworkURL = element<HTMLInputElement>("artwork-url");
  const artworkURLField = element("artwork-url-field");
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
      const label = `${sourceLabel(variant)}${variant.is_installed ? ` · ${t("Installed")}` : ""}`;
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
    artworkStatus.textContent = t("Applying artwork from the selected source…");
    void Host.CopyGameArtwork(target.id, sourceID).then((updated) => {
      if (!updated || game !== target) return;
      applyGame(updated);
      fillVariants();
      refreshCurrentArtwork();
      artworkStatus.textContent = t("Artwork applied from the selected source.");
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
    searchTitle.textContent = t("Search for {0}", label);
    queryInput.value = game ? (google ? `${game.name} ${label}` : game.name) : "";
    browserButton.hidden = !google;
    providerButton.hidden = google;
    artworkURLField.hidden = !google;
    artworkURL.disabled = !google;
    artworkURL.value = "";
    artworkSlots.forEach((slot) => slot.setAttribute("aria-pressed", String(slot.dataset.artworkField === artworkField)));
    artworkStatus.textContent = "";
  }

  function refreshPreview(): void {
    if (previewURL) URL.revokeObjectURL(previewURL);
    const file = artworkFile.files?.[0];
    previewURL = file ? URL.createObjectURL(file) : "";
    const src = previewURL || selectedImage?.image.url || selectedImage?.image.thumbnail || "";
    preview.hidden = !src;
    preview.onerror = () => {
      const thumbnail = selectedImage?.image.thumbnail;
      if (thumbnail && thumbnail !== src && preview.getAttribute("src") !== thumbnail) preview.src = thumbnail;
    };
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
    artworkURL.value = "";
    selectedImage = null;
    for (const control of imageResults.querySelectorAll("button")) control.setAttribute("aria-pressed", "false");
    refreshPreview();
  });
  artworkURL.addEventListener("input", () => {
    artworkFile.value = "";
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
    artworkStatus.textContent = sourceSelect.value === "google"
      ? t("Searching Google Images. Complete any verification in the search window.")
      : t("Searching for images…");
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
        control.setAttribute("aria-label", size ? t("Select image {0}", size) : t("Select image"));
        control.setAttribute("aria-pressed", "false");
        control.textContent = size;
        control.prepend(thumbnail);
        control.addEventListener("click", () => {
          selectedImage = { image: result, provider };
          artworkFile.value = "";
          artworkURL.value = "";
          for (const other of imageResults.querySelectorAll("button")) {
            other.setAttribute("aria-pressed", String(control === other));
          }
          refreshPreview();
          artworkStatus.textContent = t("Image selected. Choose Apply image to save it.");
        });
        imageResults.append(control);
      }
      artworkStatus.textContent = results.length ? t("{0} images found.", results.length) : t("No images found.");
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
    const params = new URLSearchParams({ udm: "2", q: query, hl: language, safe: "active" });
    if (artworkField === "logo") params.set("tbs", "ic:trans");
    void Browser.OpenURL(`https://www.google.com/search?${params}`).catch((error) => {
      artworkStatus.textContent = options.error(error);
    });
  });
  providerButton.addEventListener("click", () => {
    if (!game || busy) return;
    const target = game;
    const field = artworkField;
    busy = true;
    artworkStatus.textContent = t("Finding the highest-rated image…");
    void Host.ApplyProviderArtwork(target.id, field).then(() => Host.GetGame(target.id)).then((updated) => {
      if (!updated || game !== target) return;
      applyGame(updated);
      refreshCurrentArtwork();
      artworkStatus.textContent = t("Image applied.");
    }).catch((error) => {
      artworkStatus.textContent = options.error(error);
    }).finally(() => {
      busy = false;
    });
  });
  artworkForm.addEventListener("submit", (event) => {
    event.preventDefault();
    if (busy) return;
    const file = artworkFile.files?.[0];
    const target = game;
    const remote = selectedImage;
    const url = artworkURL.disabled ? "" : artworkURL.value.trim();
    if (!target || (!file && !remote && !url)) {
      artworkStatus.textContent = artworkURL.disabled ? t("Select a thumbnail or a file.") : t("Select a thumbnail, paste a URL, or choose a file.");
      return;
    }
    if (file && file.size > 3 * 1024 * 1024) {
      artworkStatus.textContent = t("Choose an image up to 3 MB.");
      return;
    }
    const field = artworkField;
    busy = true;
    artworkStatus.textContent = t("Applying image…");
    void (async () => {
      if (remote) {
        if (remote.provider === "google") await Host.ApplyWebImage(target.id, field, remote.image);
        else await Host.ApplySteamGridImage(target.id, field, remote.image);
      } else if (url) {
        await Host.ApplyWebImage(target.id, field, { url, thumbnail: "", width: 0, height: 0 });
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
      artworkStatus.textContent = t("Image applied.");
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
  const steamProperties = element<HTMLButtonElement>("launch-steam-properties");
  const download = element<HTMLButtonElement>("launch-proton-download");
  const refreshRunners = element<HTMLButtonElement>("launch-runner-refresh");
  const recommendation = element("launch-recommendation");
  const save = element<HTMLButtonElement>("launch-save");
  const missingPrefix = "download:";
  let runners: Runner[] = [];
  let windowsExecutable = "";
  let automaticLabel = "";
  let effectiveRuntime = "";
  let reportedVersion = "";
  let reportDetails: ProtonIntelligence | null | undefined;
  let savedPath = "";
  let launchGeneration = 0;

  function selectedRunner(): RunnerSelection {
    const value = runnerSelect.value || "auto";
    if (value === "auto" || value === "wine") return { runner: value, protonPath: "" };
    return { runner: "proton", protonPath: value };
  }

  function missingVersion(): string {
    return runnerSelect.value.startsWith(missingPrefix) ? runnerSelect.value.slice(missingPrefix.length) : "";
  }

  function updateCommandPreview(): void {
    if (!game) return;
    const missing = missingVersion();
    const choice = selectedRunner();
    const args = argumentsField.value.split("\n").map((arg) => arg.trim()).filter(Boolean);
    const suffix = args.length ? " " + args.map((arg) => JSON.stringify(arg)).join(" ") : "";
    const selectedProton = runners.find((item) => item.path === choice.protonPath);
    const automaticProton = game.plugin_id === "linux" && windowsExecutable && choice.runner === "auto"
      ? runners.find((item) => item.kind === "proton")
      : undefined;
    const protonPath = selectedProton?.path || automaticProton?.path;
    download.hidden = refreshRunners.hidden = !missing;
    save.disabled = localSettings.disabled || Boolean(missing) || (choice.runner !== "auto" && !windowsExecutable);
    if (missing) {
      download.textContent = protonDownload(missing).label;
      commandPreview.textContent = t("Waiting for installation");
      runnerHint.textContent = t("{0} was not found on this computer. Check installed runners after downloading it.", missing);
      return;
    }
    if (protonPath && windowsExecutable) {
      commandPreview.textContent = `${JSON.stringify(protonPath)} run ${JSON.stringify(windowsExecutable)}${suffix}`;
      runnerHint.textContent = t("Playnux starts the Windows executable in its own prefix. Choose Automatic to launch through the store client.");
      return;
    }
    if (windowsExecutable && (choice.runner === "wine" || game.plugin_id === "linux")) {
      commandPreview.textContent = `wine ${JSON.stringify(windowsExecutable)}${suffix}`;
      runnerHint.textContent = t("System Wine");
      return;
    }

    commandPreview.textContent = game.executable ? `${game.executable}${suffix}` : t("{0} client", sourceLabel(game));
    runnerHint.textContent = automaticLabel.startsWith("Linux nativo")
      ? t("The Linux installation uses native mode.")
      : game.plugin_id === "emulation"
        ? t("Additional arguments are passed to the emulator.")
        : !windowsExecutable && game.is_installed
          ? t("Select this installation's Windows executable to use Proton in Playnux.")
          : t("The store client manages automatic launch.");
  }

  function protonName(path: string): string {
    return path.split("/").filter(Boolean).at(-2) || t("Saved Proton");
  }

  function runnerFor(version: string): Runner | undefined {
    return runners.find((entry) => entry.kind === "proton" &&
      (runnerMatches(version, entry.name) || runnerMatches(version, entry.version ?? "")));
  }

  function fillRunners(choice = runnerSelect.value): void {
    const list = [new Option(automaticLabel ? t("Automatic · {0}", runtimeLabel(automaticLabel)) : t("Automatic · Store client"), "auto")];
    if (windowsExecutable && runners.some((runner) => runner.kind === "wine")) {
      list.push(new Option(t("System Wine"), "wine"));
    }
    for (const runner of runners) {
      if (runner.kind !== "proton" || (!windowsExecutable && runner.path !== savedPath)) continue;
      const version = runner.version && !runnerMatches(runner.version, runner.name) ? ` · ${runner.version}` : "";
      list.push(new Option(runner.name + version, runner.path));
    }

    const missing = new Set<string>();
    if (windowsExecutable && reportedVersion && !runnerFor(reportedVersion)) missing.add(reportedVersion);
    if (windowsExecutable) {
      for (const version of officialProtonDownloads.keys()) {
        if (!runnerFor(version)) missing.add(version);
      }
    }
    if (savedPath && !runners.some((entry) => entry.path === savedPath)) missing.add(protonName(savedPath));
    if (missing.size) {
      const group = document.createElement("optgroup");
      group.label = t("Available to download");
      group.append(...[...missing].map((version) => new Option(version, missingPrefix + version)));
      runnerSelect.replaceChildren(...list, group);
    } else {
      runnerSelect.replaceChildren(...list);
    }
    if (choice.startsWith(missingPrefix)) {
      choice = runnerFor(choice.slice(missingPrefix.length))?.path ?? choice;
    } else if (choice === savedPath && savedPath && !runners.some((entry) => entry.path === savedPath)) {
      choice = missingPrefix + protonName(savedPath);
    }
    runnerSelect.value = [...runnerSelect.options].some((entry) => entry.value === choice) ? choice : "auto";
    runnerSelect.disabled = runnerSelect.options.length === 1;
    updateCommandPreview();
  }

  function fillLaunch(): void {
    if (!game) return;
    const target = game;
    const generation = ++launchGeneration;
    const current = () => game === target && dialog.open && generation === launchGeneration;
    const configurable = ["linux", "emulation", "steam", "gog", "epic"].includes(target.plugin_id);
    windowsExecutable = "";
    automaticLabel = "";
    effectiveRuntime = "";
    savedPath = "";
    reportedVersion = "";
    reportDetails = undefined;
    runners = [];
    directory.value = "";
    argumentsField.value = "";
    localSettings.disabled = true;
    save.hidden = !configurable;
    save.disabled = true;
    download.hidden = refreshRunners.hidden = true;
    runnerSelect.replaceChildren(new Option(t("Loading…"), "auto"));
    runtime.replaceChildren();
    runtime.hidden = true;
    recommendation.hidden = true;
    proton.hidden = steamProperties.hidden = !hasProtonReports(target);
    launchProvider.textContent = configurable ? "" : t("Managed by {0}.", sourceLabel(target));
    launchStatus.textContent = "";
    commandPreview.textContent = "—";

    void Promise.all([
      Host.GetGameRuntime(target.id),
      Host.GetLaunchOptions(target.id),
      configurable ? Host.GetCompatibilityRunners() : [],
      configurable ? Host.GetWindowsExecutable(target.id) : "",
    ]).then(([result, settings, installed, executable]) => {
      if (!current()) return;
      automaticLabel = result.automatic || result.configured;
      effectiveRuntime = result.running || result.configured;
      if (reportDetails !== undefined) recommendation.textContent = `ProtonDB · ${recommendationText(reportDetails, effectiveRuntime)}`;
      const rows = [
        [t("Configured"), result.configured],
        [t("Running"), result.running],
        [t("Proton last used by Steam"), result.last_used],
      ];
      for (const [label, value] of rows) {
        if (!label || !value) continue;
        const term = document.createElement("dt");
        const definition = document.createElement("dd");
        term.textContent = label;
        definition.textContent = runtimeLabel(value);
        runtime.append(term, definition);
      }
      runtime.hidden = !runtime.childElementCount;
      const badge = result.running || result.configured;
      runtimeBadge.hidden = !badge;
      runtimeBadge.textContent = result.running ? t("Running · {0}", runtimeLabel(result.running)) : runtimeLabel(badge);
      if (!configurable) return;
      windowsExecutable = executable;
      runners = installed ?? [];
      directory.value = settings.directory || "";
      argumentsField.value = (settings.arguments ?? []).join("\n");
      savedPath = settings.proton_path || "";
      localSettings.disabled = false;
      fillRunners(settings.runner === "proton" ? savedPath : settings.runner || "auto");
    }).catch((error) => {
      if (current()) launchStatus.textContent = options.error(error);
    });

    if (hasProtonReports(target)) {
      void Host.GetGameCompatibility(target.id).then(({ protondb }) => {
        if (!current()) return;
        reportDetails = protondb ?? null;
        recommendation.hidden = false;
        recommendation.textContent = `ProtonDB · ${recommendationText(protondb, effectiveRuntime)}`;
        reportedVersion = protondb?.recommended ?? "";
        if (!localSettings.disabled) fillRunners();
      }).catch(() => undefined);
    }
  }

  download.addEventListener("click", () => {
    const version = missingVersion();
    if (version) void Browser.OpenURL(protonDownload(version).url).catch((error) => {
      launchStatus.textContent = options.error(error);
    });
  });
  refreshRunners.addEventListener("click", () => {
    const generation = launchGeneration;
    refreshRunners.disabled = true;
    void Host.GetCompatibilityRunners().then((installed) => {
      if (generation !== launchGeneration) return;
      runners = installed ?? [];
      fillRunners();
    }).catch((error) => {
      if (generation === launchGeneration) launchStatus.textContent = options.error(error);
    }).finally(() => { refreshRunners.disabled = false; });
  });
  runnerSelect.addEventListener("change", updateCommandPreview);
  argumentsField.addEventListener("input", updateCommandPreview);
  launchForm.addEventListener("submit", (event) => {
    event.preventDefault();
    if (!game || busy || localSettings.disabled || save.disabled || missingVersion()) return;
    const target = game;
    const choice = selectedRunner();
    const args = argumentsField.value.split("\n").map((arg) => arg.trim()).filter(Boolean);
    busy = true;
    save.disabled = true;
    launchStatus.textContent = t("Saving…");
    void Host.SaveLaunchOptions(target.id, directory.value, args, choice.runner, choice.protonPath).then(() => {
      if (game !== target) return;
      options.toast(t("Launch options updated."));
      fillLaunch();
    }).catch((error) => {
      launchStatus.textContent = options.error(error);
    }).finally(() => {
      busy = false;
      updateCommandPreview();
    });
  });
  steamProperties.addEventListener("click", () => {
    if (!game || !hasProtonReports(game)) return;
    void Browser.OpenURL(`steam://gameproperties/${game.game_id}/compatibility`).catch((error) => {
      launchStatus.textContent = options.error(error);
    });
  });
  proton.addEventListener("click", () => {
    if (!game) return;
    void openProtonReports(game).catch((error) => {
      launchStatus.textContent = options.error(error);
    });
  });

  // ── Ciclo de vida ────────────────────────────────────────────────────────
  close.addEventListener("click", () => dialog.close());
  dialog.addEventListener("close", () => {
    game = null;
    launchGeneration++;
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
      runtimeBadge.hidden = true;
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
