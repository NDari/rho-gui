<script lang="ts">
  import { onMount, tick } from "svelte";
  import { Application, Browser, Events } from "@wailsio/runtime";
  import { Bridge } from "../bindings/github.com/NDari/rho/gui/bridge";
  import type { Command } from "../bindings/github.com/NDari/rho/gui/bridge";
  import { store } from "./lib/store.svelte";
  import type { CoreEvent } from "./lib/types";
  import { humanBytes, humanTokens, oneLine, setHome, shortenHome, time } from "./lib/format";
  import ItemView from "./components/ItemView.svelte";
  import Composer, { type CommandInfo } from "./components/Composer.svelte";
  import StatusBar from "./components/StatusBar.svelte";
  import Picker, { type PickAction, type PickItem } from "./components/Picker.svelte";
  import Dialog, { type DialogSpec } from "./components/Dialog.svelte";

  interface UIRequest {
    id: number;
    kind: "confirm" | "select" | "input";
    title: string;
    message?: string;
    placeholder?: string;
    options?: string[];
  }

  interface PickerSpec {
    title: string;
    items: PickItem[];
    hint?: string;
    actions?: PickAction[];
    onpick: (it: PickItem) => void;
    onclose?: () => void;
  }

  const builtin: CommandInfo[] = [
    { name: "model", description: "switch model" },
    { name: "new", description: "start a new session" },
    { name: "resume", description: "resume a previous session" },
    { name: "rewind", description: "go back to an earlier message (conversation only)" },
    { name: "fork", description: "copy the conversation up to a message into a new session" },
    { name: "tree", description: "browse all branches of this session" },
    { name: "aside", description: "start a read-only side conversation" },
    { name: "back", description: "return from a side conversation [note | --summarize]" },
    { name: "compact", description: "summarize older messages to free context" },
    { name: "context", description: "show what is sent to the model" },
    { name: "label", description: "label the current position" },
    { name: "copy", description: "copy the last answer to the clipboard [all]" },
    { name: "theme", description: "choose light, dark or system colors" },
    { name: "config", description: "change and save settings [key value]" },
    { name: "extensions", description: "list extensions and turn them on or off" },
    { name: "reload", description: "restart extensions and reload context files" },
    { name: "help", description: "show keys and commands" },
    { name: "quit", description: "exit Rho" },
  ];

  let extraCommands: Command[] = $state([]);
  const commands = $derived(
    [...builtin, ...extraCommands.map((c) => ({ name: c.name, description: c.description }))].sort((a, b) =>
      a.name.localeCompare(b.name),
    ),
  );

  let picker: PickerSpec | null = $state(null);
  let dialog: DialogSpec | null = $state(null);
  let uiQueue: UIRequest[] = $state([]);
  let composer: ReturnType<typeof Composer> | undefined = $state();
  let scroller: HTMLDivElement | undefined = $state();
  let content: HTMLDivElement | undefined = $state();
  let follow = true;

  // Theme: "system", "light" or "dark", kept in local storage.
  let themePref = $state(localStorage.getItem("rho.theme") ?? "system");
  const dark = window.matchMedia("(prefers-color-scheme: dark)");
  function applyTheme() {
    const t = themePref === "system" ? (dark.matches ? "dark" : "light") : themePref;
    document.documentElement.dataset.theme = t;
  }
  applyTheme();
  dark.addEventListener("change", applyTheme);

  function errText(err: unknown): string {
    if (err && typeof err === "object" && "message" in err) return String((err as Error).message);
    return String(err);
  }

  async function call<T>(p: Promise<T>): Promise<T | undefined> {
    try {
      return await p;
    } catch (err) {
      store.notice("error", errText(err));
      return undefined;
    }
  }

  function closePicker() {
    const p = picker;
    picker = null;
    p?.onclose?.();
    void tick().then(() => composer?.focus());
  }

  function pick(spec: PickerSpec) {
    picker = {
      ...spec,
      onpick: (it) => {
        picker = null;
        spec.onpick(it);
        void tick().then(() => composer?.focus());
      },
    };
  }

  function prompt(title: string, placeholder: string, initial: string, done: (v: string) => void) {
    dialog = {
      kind: "input",
      title,
      placeholder,
      initial,
      respond: (v, ok) => {
        dialog = null;
        if (ok) done(v);
        void tick().then(() => composer?.focus());
      },
    };
  }

  // Extension prompts arrive one at a time.
  $effect(() => {
    const r = uiQueue[0];
    if (!r || dialog || picker) return;
    const answer = (value: string, ok: boolean) => {
      uiQueue = uiQueue.slice(1);
      void Bridge.Respond(r.id, value, ok);
    };
    if (r.kind === "select") {
      picker = {
        title: r.title,
        items: (r.options ?? []).map((o) => ({ title: o, value: o })),
        onpick: (it) => {
          picker = null;
          answer(it.value, true);
        },
        onclose: () => answer("", false),
      };
    } else {
      dialog = {
        kind: r.kind,
        title: r.title,
        message: r.message,
        placeholder: r.placeholder,
        respond: (v, ok) => {
          dialog = null;
          answer(v, ok);
        },
      };
    }
  });

  function idle(name: string): boolean {
    if (store.busy) {
      store.notice("warn", `/${name} is not available while the agent is working (Esc to cancel)`);
      return false;
    }
    return true;
  }

  async function submit(text: string, followUp: boolean) {
    follow = true;
    if (text.startsWith("/") && !text.startsWith("//")) {
      await runCommand(text);
      return;
    }
    if (text.startsWith("//")) text = text.slice(1);
    await call(Bridge.Send(text, followUp));
  }

  async function runCommand(line: string) {
    const sp = line.indexOf(" ");
    const name = (sp < 0 ? line.slice(1) : line.slice(1, sp)).trim();
    const args = sp < 0 ? "" : line.slice(sp + 1).trim();
    switch (name) {
      case "quit":
      case "exit":
      case "q":
        void Application.Quit();
        return;
      case "help":
        store.add({ kind: "text", title: "Rho help", body: helpText() });
        return;
      case "model":
        if (!idle(name)) return;
        if (args) await call(Bridge.SetModel(args));
        else await modelPicker();
        return;
      case "new":
        if (!idle(name)) return;
        store.noticeAfterReload("info", "New session");
        await call(Bridge.NewSession());
        return;
      case "resume":
        if (!idle(name)) return;
        await resumePicker();
        return;
      case "rewind":
        if (!idle(name)) return;
        await rewindPicker();
        return;
      case "fork":
        if (!idle(name)) return;
        await forkPicker();
        return;
      case "tree":
        if (!idle(name)) return;
        await treePicker();
        return;
      case "aside":
        if (!idle(name)) return;
        await call(Bridge.StartAside());
        return;
      case "back":
        if (!idle(name)) return;
        if (args === "--summarize" || args === "-s") {
          store.notice("info", "Summarizing the side conversation…");
          const note = await call(Bridge.SummarizeAside());
          if (note) prompt("Note for the main conversation", "edit the note", note, (v) => void call(Bridge.EndAside(v)));
          return;
        }
        await call(Bridge.EndAside(args));
        return;
      case "compact":
        if (!idle(name)) return;
        store.busy = true;
        await call(Bridge.Compact());
        store.busy = false;
        return;
      case "context":
        await contextReport();
        return;
      case "label":
        if (await call(Bridge.Label("", args)) !== undefined) {
          store.notice("info", args ? "Labeled: " + args : "Label removed");
        }
        return;
      case "copy": {
        const ok = await call(Bridge.Copy(args === "all"));
        if (ok === false) store.notice("warn", "Nothing to copy");
        else if (ok) store.notice("info", "Copied to the clipboard");
        return;
      }
      case "theme":
        if (args) setTheme(args);
        else themePicker();
        return;
      case "config":
        if (!idle(name)) return;
        await configCommand(args);
        return;
      case "extensions":
        if (!store.state?.extensions) {
          store.notice("info", "Extensions are turned off for this run");
          return;
        }
        if (!idle(name)) return;
        await extensionsPicker();
        return;
      case "reload":
        if (!idle(name)) return;
        await call(Bridge.Reload());
        await loadCommands();
        return;
    }
    const found = await call(Bridge.RunCommand(name, args, false));
    if (found === false) store.notice("error", `Unknown command /${name} (see /help)`);
  }

  function helpText(): string {
    let s = `**Keys**

- Enter: send · Shift+Enter or Alt+Enter: new line
- While working: Enter steers (interrupts after the current tool), Alt+Enter queues a follow-up, Esc cancels
- Ctrl+O: expand all tool output and diffs · Ctrl+T: show thinking
- ↑/↓ on the first/last line: prompt history · Tab: accept a command completion
- @path attaches a file · start a message with // to send a literal slash
- Click a tool card's header to expand or collapse it

**Commands**

`;
    for (const c of commands) s += `- /${c.name}: ${c.description}\n`;
    return s;
  }

  async function modelPicker() {
    const models = (await call(Bridge.Models())) ?? [];
    if (models.length === 0) {
      store.notice("warn", "No models configured; use /model provider/name");
      return;
    }
    const cur = store.state?.model;
    pick({
      title: "Model",
      items: models.map((m) => ({
        title: m.id,
        detail: m.provider + (m.context_window ? ` · ${humanTokens(m.context_window)} ctx` : "") + (m.id === cur ? " · current" : ""),
        value: m.id,
        current: m.id === cur,
      })),
      onpick: (it) => void call(Bridge.SetModel(it.value)),
    });
  }

  async function resumePicker() {
    const list = ((await call(Bridge.Sessions())) ?? []).filter((s) => !s.current);
    if (list.length === 0) {
      store.notice("info", "No other sessions in this directory");
      return;
    }
    pick({
      title: "Resume session",
      items: list.map((s) => ({
        title: s.title || "(untitled)",
        detail: `${time(s.modified, true)} · ${s.messages} msgs · ${humanBytes(s.size)}${s.model ? " · " + s.model : ""}`,
        value: s.path,
      })),
      onpick: (it) => void call(Bridge.Resume(it.value)),
    });
  }

  function refItems(refs: { id: string; role: string; text?: string; tool_name?: string; ts: string; label?: string }[]): PickItem[] {
    return refs.map((r) => {
      let title = oneLine(r.text ?? "");
      if (!title) title = r.tool_name ? `[tool calls: ${r.tool_name}…]` : "(empty)";
      let detail = r.role ? `${r.role} · ${time(r.ts)}` : "latest";
      if (r.label) detail = `🏷 ${r.label} · ${detail}`;
      return { title, detail, value: r.id };
    });
  }

  async function rewindPicker() {
    const refs = (await call(Bridge.RewindTargets())) ?? [];
    if (refs.length === 0) {
      store.notice("info", "Nothing to rewind");
      return;
    }
    pick({
      title: "Rewind to before…",
      hint: "The selected message goes back into the editor · Esc close",
      items: refItems(refs),
      onpick: async (it) => {
        store.noticeAfterReload("banner", "Rewound the conversation. Files on disk were NOT reverted.");
        const text = await call(Bridge.Rewind(it.value));
        if (text !== undefined) store.draft = text;
      },
    });
  }

  async function forkPicker() {
    const refs = (await call(Bridge.ForkTargets())) ?? [];
    if (refs.length === 0) {
      store.notice("info", "Nothing to fork");
      return;
    }
    pick({
      title: "Fork from (inclusive)…",
      items: refItems(refs),
      onpick: async (it) => {
        const path = await call(Bridge.Fork(it.value));
        if (path) store.notice("info", "Forked into a new session: " + shortenHome(path));
      },
    });
  }

  function treeTitle(kind: string, text: string): string {
    switch (kind) {
      case "user":
        return "› " + oneLine(text);
      case "assistant":
        return "  " + oneLine(text);
      case "compaction":
        return "⇣ compaction";
      case "aside_start":
        return "↪ side conversation";
      case "aside_note":
        return "↩ note: " + oneLine(text);
      case "model_change":
        return "⚙ model: " + text;
    }
    return text;
  }

  async function treePicker() {
    const nodes = (await call(Bridge.Tree())) ?? [];
    if (nodes.length === 0) {
      store.notice("info", "The session is empty");
      return;
    }
    const labels = new Map(nodes.map((n) => [n.id, n.label ?? ""]));
    pick({
      title: "Session tree",
      hint: "Enter check out · Esc close",
      items: nodes.map((n) => {
        let detail = time(n.ts, true);
        if (n.current) detail = "● current · " + detail;
        else if (n.on_path) detail = "│ " + detail;
        if (n.label) detail = `🏷 ${n.label} · ${detail}`;
        return { title: treeTitle(n.kind, n.text ?? ""), detail, value: n.id, depth: Math.min(n.depth, 12), current: n.current };
      }),
      actions: [
        {
          key: "ctrl+l",
          label: "label",
          run: (it) => {
            picker = null;
            prompt("Label", "label for this entry (empty to clear)", labels.get(it.value) ?? "", (v) => void call(Bridge.Label(it.value, v)));
          },
        },
      ],
      onpick: async (it) => {
        store.noticeAfterReload("banner", "Moved to another point in the conversation. Files on disk were NOT changed.");
        await call(Bridge.Checkout(it.value));
      },
    });
  }

  async function contextReport() {
    const r = await call(Bridge.Context());
    if (!r) return;
    let b = "| Part | Source | Tokens |\n|---|---|---|\n";
    for (const p of r.parts ?? []) b += `| ${p.name} | ${p.path ? shortenHome(p.path) : "built-in"} | ${humanTokens(p.tokens)} |\n`;
    b += `| Tool definitions | ${r.tools} tools | ${humanTokens(r.tool_tokens)} |\n`;
    b += `| Conversation | ${r.messages} messages${r.compacted ? " (after compaction)" : ""} | ${humanTokens(r.message_tokens)} |\n`;
    b += `| **Total (estimate)** | | **${humanTokens(r.total)}** |\n`;
    if (r.window > 0) {
      b += `\nContext window: ${humanTokens(r.window)} tokens (${Math.round((r.total * 100) / r.window)}% used). Estimates use about 4 characters per token.\n`;
    }
    if (r.skills?.length) b += `\nSkills: ${r.skills.join(", ")}\n`;
    store.add({ kind: "text", title: "Context", body: b });
  }

  function setTheme(t: string) {
    if (!["system", "light", "dark"].includes(t)) {
      store.notice("error", "Unknown theme " + t + " (use light, dark or system)");
      return;
    }
    themePref = t;
    localStorage.setItem("rho.theme", t);
    applyTheme();
  }

  function themePicker() {
    pick({
      title: "Theme",
      items: ["system", "light", "dark"].map((t) => ({ title: t, value: t, detail: t === themePref ? "current" : "", current: t === themePref })),
      onpick: (it) => setTheme(it.value),
    });
  }

  function shown(v: string) {
    return v === "" ? "default" : v;
  }

  async function setSetting(key: string, value: string): Promise<boolean> {
    const r = await call(Bridge.SetSetting(key, value));
    if (!r) return false;
    store.notice("info", `${key} = ${shown(r.value)}${r.saved ? " (saved)" : " (this session)"}${r.note ? "\n" + r.note : ""}`);
    if (key === "tui.show_thinking") store.showThinking = r.value === "true";
    await store.refreshState();
    return true;
  }

  async function configCommand(args: string) {
    if (args) {
      const sp = args.indexOf(" ");
      if (sp < 0) {
        store.notice("error", "Usage: /config KEY VALUE");
        return;
      }
      await setSetting(args.slice(0, sp), args.slice(sp + 1).trim());
      return;
    }
    const settings = (await call(Bridge.Settings())) ?? [];
    const model = store.state?.model ?? "";
    pick({
      title: "Config",
      hint: "Enter changes the value · Esc close · /config KEY VALUE sets any value",
      items: settings.map((s) => ({
        title: `${s.key} = ${shown(s.value)}`,
        detail: s.description + (s.per_model ? ` (${model})` : ""),
        value: s.key,
      })),
      onpick: async (it) => {
        const s = settings.find((x) => x.key === it.value)!;
        const choices = s.kind === "bool" ? ["true", "false"] : (s.choices ?? []);
        if (choices.length === 0) {
          prompt(s.key, s.description, s.value, (v) => void setSetting(s.key, v).then(() => configCommand("")));
          return;
        }
        pick({
          title: s.key,
          items: choices.map((c) => ({ title: shown(c), value: c, detail: c === s.value ? "current" : "", current: c === s.value })),
          onpick: (c) => void setSetting(s.key, c.value).then(() => configCommand("")),
        });
      },
    });
  }

  async function extensionsPicker() {
    const list = (await call(Bridge.Extensions())) ?? [];
    if (list.length === 0) {
      store.notice("info", "No extensions found");
      return;
    }
    pick({
      title: "Extensions",
      hint: "Enter turns the extension on or off · Esc close",
      items: list.map((x) => ({
        title: `${x.disabled ? "○" : "●"} ${x.name || x.id}`,
        detail: `${x.kind}${x.project ? " · project" : ""} · ${x.status}${x.error ? ": " + x.error : ""} · ${x.tools} tools, ${x.commands} commands, ${x.hooks} hooks`,
        value: x.id,
      })),
      onpick: async (it) => {
        const x = list.find((e) => e.id === it.value)!;
        const note = await call(Bridge.SetExtensionEnabled(x.id, !!x.disabled));
        if (note !== undefined) {
          store.notice("info", `${x.id} turned ${x.disabled ? "on" : "off"}${note ? "\n" + note : ""}`);
          await loadCommands();
        }
        await extensionsPicker();
      },
    });
  }

  async function loadCommands() {
    extraCommands = (await call(Bridge.Commands())) ?? [];
  }

  function globalKey(e: KeyboardEvent) {
    if (picker || dialog) return;
    if (e.ctrlKey && e.key === "o") {
      store.expandAll = !store.expandAll;
      e.preventDefault();
    } else if (e.ctrlKey && e.key === "t") {
      store.showThinking = !store.showThinking;
      e.preventDefault();
    } else if (e.key === "Escape" && store.busy) {
      void Bridge.Cancel();
      e.preventDefault();
    }
  }

  // Links open in the system browser, never inside the window.
  function linkClick(e: MouseEvent) {
    const a = (e.target as HTMLElement).closest("a");
    if (!a || !a.href) return;
    e.preventDefault();
    if (/^https?:|^mailto:/.test(a.href)) void Browser.OpenURL(a.href);
  }

  function onScroll() {
    if (!scroller) return;
    follow = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 40;
  }

  onMount(() => {
    const offEvents = Events.On("rho:events", (ev: { data: CoreEvent[] }) => {
      for (const e of ev.data ?? []) store.handle(e);
    });
    const offUI = Events.On("rho:ui", (ev: { data: UIRequest }) => {
      uiQueue = [...uiQueue, ev.data];
    });
    const ro = new ResizeObserver(() => {
      if (follow && scroller) scroller.scrollTop = scroller.scrollHeight;
    });
    if (content) ro.observe(content);

    void (async () => {
      setHome(await Bridge.HomeDir());
      await store.refreshState();
      store.showThinking = !!store.state?.show_thinking;
      for (const w of store.state?.warnings ?? []) store.notice("warn", w);
      await store.loadTranscript();
      await loadCommands();
      await Bridge.Ready();
      composer?.focus();
    })();

    return () => {
      offEvents();
      offUI();
      ro.disconnect();
    };
  });
</script>

<svelte:window onkeydown={globalKey} />

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<main onclick={linkClick}>
  <div class="scroller" bind:this={scroller} onscroll={onScroll}>
    <div class="transcript" bind:this={content}>
      {#if store.items.length === 0}
        <div class="empty">
          <div class="logo">ρ</div>
          <div>Ask Rho to read, write, edit or run something in <b>{shortenHome(store.state?.cwd ?? "")}</b>.</div>
          <div class="muted">Type / for commands.</div>
        </div>
      {/if}
      {#each store.items as item (item.key)}
        <ItemView {item} cwd={store.state?.cwd ?? ""} showThinking={store.showThinking} expandAll={store.expandAll} />
      {/each}
      {#if store.steering.length || store.followUps.length}
        <div class="queue">
          {#each store.steering as s, i (i)}<div>Steering: {s}</div>{/each}
          {#each store.followUps as s, i (i)}<div>Follow-up: {s}</div>{/each}
        </div>
      {/if}
    </div>
  </div>
  <Composer
    bind:this={composer}
    bind:draft={store.draft}
    busy={store.busy}
    aside={!!store.state?.aside}
    {commands}
    onsubmit={submit}
    oncancel={() => void Bridge.Cancel()}
  />
  <StatusBar
    model={store.state?.model ?? ""}
    cwd={store.state?.cwd ?? ""}
    branch={store.state?.branch ?? ""}
    contextTokens={store.contextTokens}
    contextWindow={store.contextWindow}
    cost={store.cost}
    aside={!!store.state?.aside}
    busy={store.busy}
    statuses={store.statuses}
    onmodel={() => void runCommand("/model")}
  />
</main>

{#if picker}
  <Picker
    title={picker.title}
    items={picker.items}
    hint={picker.hint}
    actions={picker.actions}
    onpick={picker.onpick}
    onclose={closePicker}
  />
{/if}
{#if dialog}
  <Dialog spec={dialog} />
{/if}

<style>
  main {
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  .scroller {
    flex: 1;
    overflow-y: auto;
  }
  .transcript {
    max-width: 920px;
    margin: 0 auto;
    padding: 18px 18px 24px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .empty {
    margin: 18vh auto 0;
    text-align: center;
    color: var(--fg-muted);
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .logo {
    font-size: 54px;
    color: var(--accent);
    line-height: 1;
    margin-bottom: 8px;
  }
  .muted {
    color: var(--fg-faint);
  }
  .queue {
    color: var(--fg-muted);
    font-size: 12.5px;
    border-left: 3px solid var(--accent);
    padding-left: 10px;
  }
</style>
