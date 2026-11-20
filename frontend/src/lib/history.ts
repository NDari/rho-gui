// Prompt history kept in local storage.

const key = "rho.history";
const max = 500;

function load(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(key) ?? "[]");
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
}

export class History {
  private entries = load();
  private pos = -1;
  private saved = "";

  add(text: string) {
    this.entries = this.entries.filter((e) => e !== text);
    this.entries.unshift(text);
    this.entries = this.entries.slice(0, max);
    localStorage.setItem(key, JSON.stringify(this.entries));
    this.pos = -1;
  }

  /** Moves through history; dir 1 is older. Returns null at the ends. */
  move(dir: 1 | -1, current: string): string | null {
    if (this.pos === -1) this.saved = current;
    const next = this.pos + dir;
    if (next < -1 || next >= this.entries.length) return null;
    this.pos = next;
    return next === -1 ? this.saved : this.entries[next];
  }

  reset() {
    this.pos = -1;
  }
}
