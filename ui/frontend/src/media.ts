import { t } from "./i18n";
import * as Host from "../bindings/github.com/playnux/ui/host/host.js";
import type { Game } from "../bindings/github.com/playnux/core/models/models.js";

function youtubeID(raw: string): string {
  try {
    const url = new URL(raw);
    const host = url.hostname.replace(/^www\./, "");
    let id = "";
    if (host === "youtu.be") id = url.pathname.split("/")[1] ?? "";
    if (["youtube.com", "m.youtube.com", "youtube-nocookie.com"].includes(host)) {
      id = url.pathname === "/watch" ? url.searchParams.get("v") ?? ""
        : /^\/(?:embed|shorts|v)\/([^/?#]+)/.exec(url.pathname)?.[1] ?? "";
    }
    return /^[a-zA-Z0-9_-]{11}$/.test(id) ? id : "";
  } catch {
    return "";
  }
}

function directVideo(raw: string): string {
  try {
    const url = new URL(raw);
    return ["http:", "https:"].includes(url.protocol) && /\.(mp4|webm|ogv|m3u8)$/i.test(url.pathname)
      ? url.href
      : "";
  } catch {
    return "";
  }
}

const steamTrailers = new Map<string, { expires: number; sources: Promise<string[]> }>();
let preferSound = localStorage.getItem("playnux:trailer-sound") !== "off";

function steamTrailerSources(game: Game, highQuality: boolean): Promise<string[]> {
  if (game.plugin_id !== "steam") return Promise.resolve([]);
  const key = `${game.game_id}:${highQuality}`;
  const cached = steamTrailers.get(key);
  if (cached && cached.expires > Date.now()) return cached.sources;
  const sources = Host.GetTrailerSources(game.id, highQuality).then((urls) => (urls ?? []).filter(Boolean))
    .catch(() => { steamTrailers.delete(key); return []; });
  steamTrailers.set(key, { expires: Date.now() + 10 * 60_000, sources });
  return sources;
}

/** A background stays transparent until a video is actually playing. */
export function backgroundTrailer(container: HTMLElement, soundButton: HTMLButtonElement, highQuality = false) {
  let active = "";
  let generation = 0;
  let cleanup = () => {};
  let changeSound: ((enabled: boolean) => void) | undefined;
  let soundPlaying = false;

  function showSound(enabled: boolean): void {
    soundPlaying = enabled;
    soundButton.hidden = false;
    soundButton.textContent = enabled ? t("Mute trailer") : t("Unmute trailer");
  }

  soundButton.addEventListener("click", () => {
    preferSound = !soundPlaying;
    localStorage.setItem("playnux:trailer-sound", preferSound ? "on" : "off");
    changeSound?.(preferSound);
  });

  function clear(): void {
    cleanup();
    cleanup = () => {};
    changeSound = undefined;
    soundButton.hidden = true;
    container.classList.remove("trailer-playing");
    container.replaceChildren();
  }

  function stop(): void {
    active = "";
    generation++;
    clear();
  }

  function set(game: Game | null, enabled: boolean): void {
    if (!game || !enabled || document.hidden || document.documentElement.hasAttribute("data-deep-sleep")) {
      stop();
      return;
    }
    const raw = game.trailer_url || "";
    const direct = directVideo(raw);
    const youtube = youtubeID(raw);
    if (!direct && !youtube && game.plugin_id !== "steam") {
      stop();
      return;
    }
    const key = `${game.id}\n${raw}`;
    if (key === active) return;
    stop();
    active = key;
    const turn = generation;

    async function showYouTube(): Promise<void> {
      if (!youtube || turn !== generation) return;
      try {
        const source = await Host.GetYouTubeBackgroundURL(youtube);
        if (turn !== generation) return;
        const frame = document.createElement("iframe");
        frame.title = "Trailer";
        frame.tabIndex = -1;
        frame.inert = true;
        frame.allow = "autoplay; encrypted-media";
        frame.sandbox.add("allow-scripts", "allow-same-origin");
        const embed = new URL(source);
        if (preferSound) embed.searchParams.set("sound", "1");
        const origin = embed.origin;
        changeSound = (enabled) => frame.contentWindow?.postMessage({ trailerSound: enabled }, origin);
        const timer = window.setTimeout(clear, 12000);
        function receive(event: MessageEvent): void {
          if (event.source !== frame.contentWindow || event.origin !== origin || turn !== generation) return;
          if (event.data === "playnux-trailer-playing") {
            window.clearTimeout(timer);
            container.classList.add("trailer-playing");
          } else if (event.data === "playnux-trailer-idle") {
            container.classList.remove("trailer-playing");
          } else if (event.data === "playnux-trailer-sound-on" || event.data === "playnux-trailer-sound-off") {
            showSound(event.data === "playnux-trailer-sound-on");
          } else if (event.data === "playnux-trailer-error") {
            clear();
          }
        }
        window.addEventListener("message", receive);
        cleanup = () => {
          window.clearTimeout(timer);
          window.removeEventListener("message", receive);
        };
        frame.src = embed.href;
        container.replaceChildren(frame);
      } catch {
        // The artwork remains visible while media is unavailable.
      }
    }

    function playSources(sources: string[], index = 0): void {
      if (turn !== generation) return;
      clear();
      if (index >= sources.length) {
        void showYouTube();
        return;
      }
      const video = document.createElement("video");
      video.muted = !preferSound;
      video.playsInline = true;
      video.loop = true;
      const next = () => playSources(sources, index + 1);
      const timer = window.setTimeout(next, 8000);
      function playing(): void {
        if (turn !== generation) return;
        window.clearTimeout(timer);
        container.classList.add("trailer-playing");
        showSound(!video.muted);
      }
      function play(): void {
        void video.play().catch((error: DOMException) => {
          if (turn !== generation || video.parentElement !== container) return;
          if (error.name === "NotAllowedError" && !video.muted) {
            video.muted = true;
            showSound(false);
            play();
          } else if (error.name !== "AbortError") {
            next();
          }
        });
      }
      changeSound = (enabled) => {
        video.muted = !enabled;
        showSound(enabled);
        if (video.paused) play();
      };
      cleanup = () => {
        window.clearTimeout(timer);
        video.removeEventListener("error", next);
        video.removeEventListener("playing", playing);
        video.pause();
        video.removeAttribute("src");
        video.load();
      };
      video.addEventListener("error", next, { once: true });
      video.addEventListener("playing", playing);
      video.src = sources[index];
      container.replaceChildren(video);
      play();
    }

    const timer = window.setTimeout(() => {
      if (direct) {
        playSources([direct]);
      } else {
        void steamTrailerSources(game, highQuality).then((urls) => playSources([...new Set(urls)]));
      }
    }, 650);
    cleanup = () => window.clearTimeout(timer);
  }

  return { set, stop };
}
