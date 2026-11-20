<script lang="ts" module>
  export interface CommandInfo {
    name: string;
    description: string;
  }
</script>

<script lang="ts">
  import { History } from "../lib/history";

  let {
    busy,
    aside,
    commands,
    draft = $bindable(),
    onsubmit,
    oncancel,
  }: {
    busy: boolean;
    aside: boolean;
    commands: CommandInfo[];
    draft: string | null;
    onsubmit: (text: string, followUp: boolean) => void;
    oncancel: () => void;
  } = $props();

  let value = $state("");
  let el: HTMLTextAreaElement | undefined = $state();
  let compCursor = $state(0);
  const history = new History();

  export function focus() {
    el?.focus();
  }

  // Put drafts (rewound messages, leftover queue) into the editor.
  $effect(() => {
    if (draft != null) {
      if (!value.trim()) value = draft;
      draft = null;
      queueMicrotask(() => {
        resize();
        el?.focus();
      });
    }
  });

  const completions = $derived.by(() => {
    const m = /^\/(\S*)$/.exec(value);
    if (!m) return [];
    return commands.filter((c) => c.name.startsWith(m[1])).slice(0, 12);
  });

  $effect(() => {
    void completions;
    compCursor = 0;
  });

  function resize() {
    if (!el) return;
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, window.innerHeight * 0.4) + "px";
  }

  function accept(c: CommandInfo) {
    value = "/" + c.name + " ";
    queueMicrotask(resize);
  }

  function submit(followUp: boolean) {
    const text = value.trim();
    if (!text) return;
    history.add(text);
    value = "";
    queueMicrotask(resize);
    onsubmit(text, followUp);
  }

  function key(e: KeyboardEvent) {
    const comp = completions;
    if (comp.length > 0) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        compCursor = (compCursor + (e.key === "ArrowDown" ? 1 : comp.length - 1)) % comp.length;
        e.preventDefault();
        return;
      }
      if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey && value !== "/" + comp[compCursor].name)) {
        accept(comp[compCursor]);
        e.preventDefault();
        return;
      }
    }
    if (e.key === "Enter" && !e.shiftKey && !(e.ctrlKey || e.metaKey)) {
      if (e.altKey && !busy) return; // Alt+Enter adds a line when idle
      e.preventDefault();
      submit(e.altKey);
      return;
    }
    if (e.key === "Enter" && e.ctrlKey) return;
    if (e.key === "Escape") {
      if (busy) {
        e.preventDefault();
        oncancel();
      }
      return;
    }
    if (e.key === "ArrowUp" || e.key === "ArrowDown") {
      const before = value.slice(0, el!.selectionStart);
      const after = value.slice(el!.selectionEnd);
      if ((e.key === "ArrowUp" && before.includes("\n")) || (e.key === "ArrowDown" && after.includes("\n"))) {
        return;
      }
      const h = history.move(e.key === "ArrowUp" ? 1 : -1, value);
      if (h != null) {
        value = h;
        e.preventDefault();
        queueMicrotask(resize);
      }
    }
  }

  const placeholder = $derived(
    busy
      ? "Enter steers · Alt+Enter queues a follow-up · Esc cancels"
      : aside
        ? "Side conversation (read-only) · /back to return"
        : "Message Rho · / for commands · @path attaches a file",
  );
</script>

<div class="composer" class:busy>
  {#if completions.length > 0}
    <div class="completions">
      {#each completions as c, i (c.name)}
        <button class="comp" class:sel={i === compCursor} onmousedown={(e) => (e.preventDefault(), accept(c))}>
          <span class="n">/{c.name}</span><span class="d">{c.description}</span>
        </button>
      {/each}
    </div>
  {/if}
  <textarea
    bind:this={el}
    bind:value
    rows="1"
    {placeholder}
    oninput={() => (history.reset(), resize())}
    onkeydown={key}
    spellcheck="false"
  ></textarea>
  {#if busy}
    <button class="stop" onclick={oncancel} title="Cancel (Esc)">Stop</button>
  {:else}
    <button class="primary send" onclick={() => submit(false)} disabled={!value.trim()}>Send</button>
  {/if}
</div>

<style>
  .composer {
    position: relative;
    display: flex;
    gap: 8px;
    align-items: flex-end;
    padding: 10px 14px 12px;
    border-top: 1px solid var(--border);
    background: var(--bg);
  }
  textarea {
    flex: 1;
    resize: none;
    min-height: 38px;
    padding: 8px 10px;
    line-height: 1.45;
    border-radius: 10px;
  }
  .busy textarea {
    border-color: var(--accent);
  }
  .send,
  .stop {
    height: 38px;
    border-radius: 10px;
  }
  .send:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .completions {
    position: absolute;
    left: 14px;
    right: 14px;
    bottom: 100%;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 -6px 24px rgba(0, 0, 0, 0.25);
    overflow: hidden;
  }
  .comp {
    display: flex;
    gap: 12px;
    width: 100%;
    border: none;
    border-radius: 0;
    background: none;
    text-align: left;
    padding: 4px 10px;
  }
  .comp.sel {
    background: var(--bg-soft);
  }
  .n {
    font-family: var(--mono);
    color: var(--accent);
  }
  .d {
    color: var(--fg-muted);
  }
</style>
