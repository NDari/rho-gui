// Markdown rendering with syntax highlighting. Raw HTML in model output is
// escaped (html: false) and markdown-it rejects javascript: links.

import MarkdownIt from "markdown-it";
import hljs from "highlight.js/lib/common";

export function highlight(code: string, lang: string): string {
  if (lang && hljs.getLanguage(lang)) {
    try {
      return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value;
    } catch {
      // fall through to plain text
    }
  }
  return escapeHTML(code);
}

const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: false,
  highlight: (code, lang) => `<pre class="code"><code class="hljs">${highlight(code, lang)}</code></pre>`,
});

export function renderMarkdown(src: string): string {
  return md.render(src);
}

export function escapeHTML(s: string): string {
  return s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}

const extLang: Record<string, string> = {
  go: "go", ts: "typescript", tsx: "typescript", js: "javascript", jsx: "javascript", mjs: "javascript",
  py: "python", rs: "rust", rb: "ruby", java: "java", kt: "kotlin", c: "c", h: "c", cc: "cpp", cpp: "cpp",
  hpp: "cpp", cs: "csharp", swift: "swift", php: "php", sh: "bash", bash: "bash", zsh: "bash", fish: "bash",
  json: "json", yaml: "yaml", yml: "yaml", toml: "ini", ini: "ini", md: "markdown", html: "xml", xml: "xml",
  svelte: "xml", vue: "xml", css: "css", scss: "scss", sql: "sql", lua: "lua", mod: "go", diff: "diff",
  makefile: "makefile",
};

/** Guesses a highlight.js language from a file path. */
export function langForPath(path: string): string {
  const base = path.split(/[\\/]/).pop()?.toLowerCase() ?? "";
  if (base === "makefile") return "makefile";
  if (base === "dockerfile") return "dockerfile";
  const ext = base.includes(".") ? base.split(".").pop()! : "";
  return extLang[ext] ?? "";
}
