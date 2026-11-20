<script lang="ts" module>
  export interface PickItem {
    title: string;
    detail?: string;
    value: string;
    depth?: number;
    current?: boolean;
  }
  export interface PickAction {
    key: string;
    label: string;
    run: (it: PickItem) => void;
  }
</script>

<script lang="ts">
  import { onMount, tick } from "svelte";

  let {
    title,
    items,
    hint = "",
    actions = [],
    onpick,
    onclose,
  }: {
    title: string;
    items: PickItem[];
    hint?: string;
    actions?: PickAction[];
    onpick: (it: PickItem) => void;
    onclose: () => void;
  } = $props();

  let query = $state("");
  // svelte-ignore state_referenced_locally
  let cursor = $state(Math.max(0, items.findIndex((it) => it.current)));
  let inputEl: HTMLInputElement | undefined = $state();
  let listEl: HTMLDivElement | undefined = $state();

  const filtered = $derived.by(() => {
    const q = query.toLowerCase().trim();
    if (!q) return items;
    return items.filter((it) => (it.title + " " + (it.detail ?? "")).toLowerCase().includes(q));
  });

  $effect(() => {
    void query;
    cursor = Math.min(cursor, Math.max(0, filtered.length - 1));
  });

  async function scrollToCursor() {
    await tick();
    listEl?.querySelector(".row.sel")?.scrollIntoView({ block: "nearest" });
  }

  onMount(() => {
    inputEl?.focus();
    void scrollToCursor();
  });

  function key(e: KeyboardEvent) {
    const it = filtered[cursor];
    if (e.key === "ArrowDown" || (e.ctrlKey && e.key === "n")) {
      cursor = Math.min(cursor + 1, filtered.length - 1);
      void scrollToCursor();
    } else if (e.key === "ArrowUp" || (e.ctrlKey && e.key === "p")) {
      cursor = Math.max(cursor - 1, 0);
      void scrollToCursor();
    } else if (e.key === "Enter") {
      if (it) onpick(it);
    } else if (e.key === "Escape") {
      onclose();
    } else {
      const a = actions.find((a) => a.key === (e.ctrlKey ? "ctrl+" : "") + e.key.toLowerCase());
      if (!a || !it) return;
      a.run(it);
    }
    e.preventDefault();
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onclose}>
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="picker" onclick={(e) => e.stopPropagation()}>
    <div class="title">{title}</div>
    <input
      bind:this={inputEl}
      bind:value={query}
      placeholder="Type to filter"
      onkeydown={(e) => {
        if (["ArrowDown", "ArrowUp", "Enter", "Escape"].includes(e.key) || e.ctrlKey) key(e);
      }}
    />
    <div class="list" bind:this={listEl}>
      {#each filtered as it, i (it.value + ":" + i)}
        <button
          class="row"
          class:sel={i === cursor}
          style:padding-left={10 + (it.depth ?? 0) * 14 + "px"}
          onmouseenter={() => (cursor = i)}
          onclick={() => onpick(it)}
        >
          <span class="t">{it.title}</span>
          {#if it.detail}<span class="d">{it.detail}</span>{/if}
        </button>
      {:else}
        <div class="empty">No matches</div>
      {/each}
    </div>
    <div class="hint">
      {hint || "↑/↓ move · Enter choose · Esc close"}{#each actions as a (a.key)}{" · " + a.key + " " + a.label}{/each}
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.35);
    display: flex;
    justify-content: center;
    align-items: flex-start;
    padding-top: 10vh;
    z-index: 10;
  }
  .picker {
    width: min(720px, 92vw);
    max-height: 70vh;
    display: flex;
    flex-direction: column;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.35);
    overflow: hidden;
  }
  .title {
    font-weight: 600;
    padding: 10px 12px 6px;
  }
  input {
    margin: 0 12px 8px;
  }
  .list {
    overflow-y: auto;
    flex: 1;
  }
  .row {
    display: flex;
    gap: 12px;
    width: 100%;
    text-align: left;
    border: none;
    border-radius: 0;
    background: none;
    padding: 5px 12px;
    align-items: baseline;
  }
  .row.sel {
    background: var(--bg-soft);
  }
  .t {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .d {
    color: var(--fg-muted);
    font-size: 12.5px;
    white-space: nowrap;
  }
  .empty {
    padding: 10px 12px;
    color: var(--fg-muted);
  }
  .hint {
    border-top: 1px solid var(--border);
    color: var(--fg-muted);
    font-size: 12px;
    padding: 6px 12px;
  }
</style>
