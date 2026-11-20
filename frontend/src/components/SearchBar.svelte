<script lang="ts">
  import { onMount } from "svelte";

  // Finds text in the transcript (Ctrl+F). Matches are highlighted with the
  // CSS Custom Highlight API where the webview has it; otherwise the
  // current match is selected. Collapsed tool output is not searched.
  let {
    root,
    scroller,
    initial = "",
    onclose,
  }: { root: HTMLElement; scroller: HTMLElement; initial?: string; onclose: () => void } = $props();

  // svelte-ignore state_referenced_locally
  let query = $state(initial);
  let matches: Range[] = $state([]);
  let index = $state(0);
  let input: HTMLInputElement | undefined = $state();

  const highlights = "highlights" in CSS && typeof Highlight !== "undefined";

  export function focus() {
    input?.focus();
    input?.select();
  }

  function find(): Range[] {
    const q = query.toLowerCase();
    if (!q) return [];
    const out: Range[] = [];
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const text = (n.nodeValue ?? "").toLowerCase();
      for (let i = text.indexOf(q); i >= 0; i = text.indexOf(q, i + q.length)) {
        const r = document.createRange();
        r.setStart(n, i);
        r.setEnd(n, i + q.length);
        if (r.getClientRects().length > 0) out.push(r);
      }
    }
    return out;
  }

  function paint(scroll: boolean) {
    const cur = matches[index];
    if (highlights) {
      CSS.highlights.set("rho-search", new Highlight(...matches));
      if (cur) CSS.highlights.set("rho-search-current", new Highlight(cur));
      else CSS.highlights.delete("rho-search-current");
    }
    if (!cur || !scroll) return;
    if (!highlights) {
      const sel = window.getSelection();
      sel?.removeAllRanges();
      sel?.addRange(cur);
    }
    const r = cur.getBoundingClientRect();
    const s = scroller.getBoundingClientRect();
    if (r.top < s.top + 40 || r.bottom > s.bottom - 40) {
      scroller.scrollTop += r.top - s.top - s.height / 3;
    }
  }

  function run(scroll = true) {
    matches = find();
    if (index >= matches.length) index = 0;
    paint(scroll);
  }

  function step(d: number) {
    if (matches.length === 0) return;
    index = (index + d + matches.length) % matches.length;
    paint(true);
  }

  function clear() {
    if (highlights) {
      CSS.highlights.delete("rho-search");
      CSS.highlights.delete("rho-search-current");
    }
  }

  function key(e: KeyboardEvent) {
    if (e.key === "Enter") {
      step(e.shiftKey ? -1 : 1);
      e.preventDefault();
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      onclose();
    }
  }

  onMount(() => {
    focus();
    if (query) run();
    // Search again as the transcript changes, without jumping around.
    let timer: ReturnType<typeof setTimeout> | undefined;
    const mo = new MutationObserver(() => {
      clearTimeout(timer);
      timer = setTimeout(() => run(false), 150);
    });
    mo.observe(root, { childList: true, subtree: true, characterData: true });
    return () => {
      mo.disconnect();
      clearTimeout(timer);
      clear();
    };
  });
</script>

<div class="search">
  <input
    bind:this={input}
    bind:value={query}
    oninput={() => {
      index = 0;
      run();
    }}
    onkeydown={key}
    placeholder="Find in conversation"
    spellcheck="false"
  />
  <span class="count">{query ? (matches.length ? `${index + 1}/${matches.length}` : "No matches") : ""}</span>
  <button onclick={() => step(-1)} disabled={!matches.length} title="Previous (Shift+Enter)">↑</button>
  <button onclick={() => step(1)} disabled={!matches.length} title="Next (Enter)">↓</button>
  <button onclick={onclose} title="Close (Esc)">✕</button>
</div>

<style>
  .search {
    position: absolute;
    top: 10px;
    right: 18px;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 8px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.25);
  }
  input {
    width: 220px;
    padding: 4px 8px;
  }
  .count {
    min-width: 70px;
    color: var(--fg-muted);
    font-size: 12.5px;
    text-align: right;
  }
  button {
    padding: 2px 8px;
  }
  button:disabled {
    opacity: 0.5;
  }
</style>
