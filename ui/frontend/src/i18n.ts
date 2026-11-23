import brazilian from "./locales/pt-BR.json";

const key = "playnux:language";
export const language = localStorage.getItem(key) === "pt-BR" ? "pt-BR" : "en";
const messages: Record<string, string> = brazilian;

export function setLanguage(value: string): void {
  localStorage.setItem(key, value === "pt-BR" ? "pt-BR" : "en");
}

export function t(english: string, ...values: unknown[]): string {
  const message = language === "pt-BR" && Object.hasOwn(messages, english) ? messages[english] : english;
  return message.replace(/\{(\d+)\}/g, (placeholder, index: string) =>
    Number(index) < values.length ? String(values[Number(index)]) : placeholder);
}

export function translateStatic(root: ParentNode & Node): void {
  if (language === "en") return;
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const raw = node.nodeValue || "";
    const label = raw.trim().replace(/\s+/g, " ");
    if (label && Object.hasOwn(messages, label)) node.nodeValue = raw.replace(raw.trim(), () => t(label));
  }
  for (const element of root.querySelectorAll("[aria-label], [alt], [placeholder], [title]")) {
    for (const attribute of ["aria-label", "alt", "placeholder", "title"]) {
      const value = element.getAttribute(attribute);
      if (value && Object.hasOwn(messages, value)) element.setAttribute(attribute, t(value));
    }
  }
}
