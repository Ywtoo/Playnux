type ArtworkRole = "icon" | "cover" | "banner" | "big-banner" | "logo" | "logo-hd" | "carousel" | "carousel-hd";

const originalSources = new WeakMap<HTMLImageElement, string>();

export function setArtworkImage(image: HTMLImageElement, raw: string | null | undefined, role: ArtworkRole): void {
  const source = raw?.trim() ?? "";
  const current = originalSources.get(image);
  if (current === source && (!source || image.hasAttribute("src"))) return;
  if (current === undefined) {
    image.addEventListener("error", (event) => {
      const original = originalSources.get(image);
      if (image.getAttribute("src")?.startsWith("/artwork/thumb/") && original) {
        event.stopImmediatePropagation();
        image.src = original;
      }
    }, true);
  }
  originalSources.set(image, source);
  if (!source) {
    image.removeAttribute("src");
    return;
  }
  const variant = (role === "cover" || role === "banner") && devicePixelRatio > 1.25 ? role + "-hd" : role;
  image.src = /^(https:\/\/|\/artwork\/)/i.test(source)
    ? `/artwork/thumb/${variant}?src=${encodeURIComponent(source)}` : source;
}

export function sampleBackdrop(image: HTMLImageElement, container: HTMLElement): void {
  delete container.dataset.backdrop;
  if (!image.naturalWidth || !image.naturalHeight) return;
  try {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 8;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.drawImage(image,
      image.naturalWidth * .12, image.naturalHeight * .2,
      image.naturalWidth * .5, image.naturalHeight * .6, 0, 0, 8, 8);
    const { data } = ctx.getImageData(0, 0, 8, 8);
    let brightness = 0;
    for (let index = 0; index < data.length; index += 4) {
      brightness += data[index] * .2126 + data[index + 1] * .7152 + data[index + 2] * .0722;
    }
    container.dataset.backdrop = brightness / 64 >= 158 ? "light" : "dark";
  } catch {
    // Images from uncacheable external hosts still get an opaque badge.
  }
}
