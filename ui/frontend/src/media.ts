export function youtubeID(raw: string): string {
  if (!raw) {
    return "";
  }
  try {
    const url = new URL(raw);
    const host = url.hostname.replace(/^www\./, "");
    if (host === "youtu.be") {
      return url.pathname.split("/").filter(Boolean)[0] || "";
    }
    if (host !== "youtube.com" && host !== "m.youtube.com" && host !== "youtube-nocookie.com") {
      return "";
    }
    if (url.pathname === "/watch") {
      return url.searchParams.get("v") || "";
    }
    const match = /^\/(?:embed|shorts|v)\/([^/?#]+)/.exec(url.pathname);
    return match ? match[1] : "";
  } catch {
    return "";
  }
}

export interface TrailerOptions {
  autoplay?: boolean;
  muted?: boolean;
  background?: boolean;
}

export function trailerEmbedURL(raw: string, options: TrailerOptions = {}): string {
  const id = youtubeID(raw);
  if (!id) {
    return "";
  }
  const params = new URLSearchParams({
    autoplay: options.autoplay === false ? "0" : "1",
    rel: "0",
  });
  if (options.muted) {
    params.set("mute", "1");
  }
  if (options.background) {
    params.set("controls", "0");
    params.set("loop", "1");
    params.set("playlist", id);
    params.set("modestbranding", "1");
    params.set("playsinline", "1");
  }
  return `https://www.youtube.com/embed/${encodeURIComponent(id)}?${params.toString()}`;
}
