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
    files,
    onsubmit,
    oncancel,
  }: {
    busy: boolean;
    aside: boolean;
    commands: CommandInfo[];
    draft: string | null;
    files: (partial: string) => Promise<string[] | null>;
    onsubmit: (text: string, followUp: boolean) => void;
    oncancel: () => void;
  } = $props();

  // A completion replaces the text from `start` to the caret with `insert`.
  interface Completion {
    label: string;
    detail: string;
    insert: string;
    start: number;
  }

  let value = $state("");
  let caret = $state(0);
  let el: HTMLTextAreaElement | undefined = $state();
  let compCursor = $state(0);
  // dismissed hides completions (Esc) until the text changes.
  let dismissed = $state(false);
  let fileComps: Completion[] = $state([]);
  let fileSeq = 0;
  const history = new History();

  export function focus() {
    el?.focus();
  }

  // Put drafts (rewound messages, leftover queue) into the editor.
  $effect(() => {
    if (draft != null) {
      if (!value.trim()) {
        value = draft;
        caret = value.length;
      }
      draft = null;
      queueMicrotask(() => {
        resize();
        el?.focus();
      });
    }
  });

  // The @path being typed before the caret, if any.
  const fileToken = $derived.by(() => {
    const m = /(?:^|\s)@(\S*)$/.exec(value.slice(0, caret));
    return m ? { partial: m[1], start: caret - m[1].length - 1 } : null;
  });

  $effect(() => {
    const tok = fileToken;
    const seq = ++fileSeq;
    if (!tok) {
      fileComps = [];
      return;
    }
    void files(tok.partial).then((list) => {
      if (seq !== fileSeq) return;
      fileComps = (list ?? []).map((p) => {
        const dir = p.endsWith("/");
        return { label: "@" + p, detail: dir ? "folder" : "", insert: "@" + p + (dir ? "" : " "), start: tok.start };
      });
    });
  });

  const completions: Completion[] = $derived.by(() => {
    if (dismissed) return [];
    const m = /^\/(\S*)$/.exec(value);
    if (m) {
      return commands
        .filter((c) => c.name.startsWith(m[1]))
        .slice(0, 12)
        .map((c) => ({ label: "/" + c.name, detail: c.description, insert: "/" + c.name + " ", start: 0 }));
    }
    return fileComps.slice(0, 12);
  });

  $effect(() => {
    void completions;
    compCursor = 0;
  });

  function syncCaret() {
    if (el) caret = el.selectionStart;
  }

  function resize() {
    if (!el) return;
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, window.innerHeight * 0.4) + "px";
  }

  function accept(c: Completion) {
    const end = c.start === 0 && c.label.startsWith("/") ? value.length : caret;
    value = value.slice(0, c.start) + c.insert + value.slice(end);
    const pos = c.start + c.insert.length;
    caret = pos;
    queueMicrotask(() => {
      el?.setSelectionRange(pos, pos);
      // A keyup may have read the caret before the text changed.
      caret = pos;
      resize();
    });
  }

  function submit(followUp: boolean) {
    const text = value.trim();
    if (!text) return;
    history.add(text);
    value = "";
    caret = 0;
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
      const c = comp[compCursor];
      const exact = c.label.startsWith("/") && value === c.label;
      if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey && !e.altKey && !exact)) {
        accept(c);
        e.preventDefault();
        return;
      }
      if (e.key === "Escape" && !busy) {
        dismissed = true;
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
        caret = h.length;
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
      {#each completions as c, i (c.label)}
        <button class="comp" class:sel={i === compCursor} onmousedown={(e) => (e.preventDefault(), accept(c))}>
          <span class="n">{c.label}</span><span class="d">{c.detail}</span>
        </button>
      {/each}
    </div>
  {/if}
  <textarea
    bind:this={el}
    bind:value
    rows="1"
    {placeholder}
    oninput={() => (history.reset(), (dismissed = false), syncCaret(), resize())}
    onkeydown={key}
    onkeyup={syncCaret}
    onclick={syncCaret}
    onselect={syncCaret}
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
