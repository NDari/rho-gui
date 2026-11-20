// Small formatting helpers shared by components.

let home = "";

export function setHome(h: string) {
  home = h;
}

/** Replaces the home directory with ~. */
export function shortenHome(p: string): string {
  if (home && (p === home || p.startsWith(home + "/") || p.startsWith(home + "\\"))) {
    return "~" + p.slice(home.length);
  }
  return p;
}

export function relPath(cwd: string, p: string): string {
  if (cwd && (p.startsWith(cwd + "/") || p.startsWith(cwd + "\\"))) return p.slice(cwd.length + 1);
  return shortenHome(p);
}

export function humanTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, "") + "M";
  if (n >= 1000) return (n / 1000).toFixed(n >= 10_000 ? 0 : 1).replace(/\.0$/, "") + "k";
  return String(n);
}

export function humanBytes(n: number): string {
  if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + " MB";
  if (n >= 1 << 10) return Math.round(n / (1 << 10)) + " KB";
  return n + " B";
}

export function duration(ms: number): string {
  if (ms < 1000) return Math.round(ms) + "ms";
  if (ms < 60_000) return (ms / 1000).toFixed(1) + "s";
  return Math.floor(ms / 60_000) + "m" + Math.round((ms % 60_000) / 1000) + "s";
}

export function oneLine(s: string): string {
  return s.split(/\s+/).filter(Boolean).join(" ");
}

export function time(ts: string | Date, withDate = false): string {
  const d = new Date(ts);
  const t = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  return withDate ? d.toLocaleDateString([], { month: "short", day: "numeric" }) + " " + t : t;
}
