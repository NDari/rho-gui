// Frontend state: the transcript and status, updated from core events.

import { Bridge } from "../../bindings/github.com/NDari/rho-gui/bridge";
import type { State } from "../../bindings/github.com/NDari/rho-gui/bridge";
import type { CoreEvent, Item, Message, NewItem, ToolState } from "./types";

const maxToolOutput = 256 * 1024;

function resultText(content?: { type: string; text?: string }[]): string {
  return (content ?? [])
    .filter((b) => b.type === "text")
    .map((b) => b.text ?? "")
    .join("");
}

/** Splits a user message into its text and attached @file paths. */
function userText(msg: Message): { text: string; attached: string[] } {
  let text = "";
  const attached: string[] = [];
  (msg.content ?? []).forEach((b, i) => {
    if (b.type !== "text") return;
    const t = b.text ?? "";
    if (i === 0) {
      text = t;
    } else if (t.startsWith("<file path=")) {
      const m = /^<file path="([^"]*)"/.exec(t);
      if (m) attached.push(m[1]);
    } else {
      text += "\n" + t;
    }
  });
  return { text, attached };
}

export class Store {
  items: Item[] = $state([]);
  busy = $state(false);
  state: State | null = $state(null);
  contextTokens = $state(0);
  contextWindow = $state(0);
  cost = $state(0);
  statuses: Record<string, string> = $state({});
  steering: string[] = $state([]);
  followUps: string[] = $state([]);
  showThinking = $state(false);
  expandAll = $state(false);
  /** Text to put back into the composer (rewind, leftover queue). */
  draft: string | null = $state(null);

  private nextKey = 1;
  private tools = new Map<string, ToolState>();
  private cur: Extract<Item, { kind: "assistant" }> | null = null;
  private compaction: Extract<Item, { kind: "compaction" }> | null = null;
  private postReload: NewItem[] = [];

  add(it: NewItem): Item {
    const item = { ...it, key: this.nextKey++ } as Item;
    this.items.push(item);
    // Return the reactive proxy so later changes update the view.
    return this.items[this.items.length - 1];
  }

  notice(level: string, text: string) {
    this.add({ kind: "notice", level, text });
  }

  /** Shows a notice after the next transcript reload. */
  noticeAfterReload(level: string, text: string) {
    this.postReload.push({ kind: "notice", level, text });
  }

  async refreshState() {
    const s = await Bridge.State();
    this.state = s;
    this.busy = s.busy;
    this.contextTokens = s.context_tokens;
    this.contextWindow = s.context_window;
    this.cost = s.session_cost;
  }

  /** Rebuilds the transcript from the session. */
  async loadTranscript() {
    const t = await Bridge.Transcript();
    // Startup warnings stay until the first real content.
    const keep = this.items.filter(
      (it) => it.kind === "notice" && it.level === "warn" && this.tools.size === 0 && !this.cur,
    );
    this.items = keep;
    this.tools.clear();
    this.cur = null;
    this.compaction = null;
    for (const it of t.items ?? []) {
      switch (it.kind) {
        case "user":
          this.add({ kind: "user", text: it.text ?? "", attached: it.attached ?? [] });
          break;
        case "assistant":
          this.add({
            kind: "assistant",
            text: it.text ?? "",
            thinking: it.thinking ?? "",
            cancelled: !!it.cancelled,
            streaming: false,
            composing: "",
          });
          break;
        case "tool": {
          const tc = it.tool!;
          const item = this.add({
            kind: "tool",
            tool: {
              id: tc.id,
              name: tc.name,
              args: tc.args,
              running: false,
              output: "",
              result: tc.result ?? "",
              isError: !!tc.is_error,
              display: tc.display ?? null,
              durationMs: 0,
            },
          });
          if (item.kind === "tool") this.tools.set(tc.id, item.tool);
          break;
        }
        case "compaction":
          this.add({
            kind: "compaction",
            running: false,
            reason: "",
            before: it.tokens_before ?? 0,
            after: 0,
            summary: it.summary ?? "",
            error: "",
          });
          break;
        case "note":
          this.notice("info", it.text ?? "");
          break;
      }
    }
    if (t.aside) {
      this.notice(
        "warn",
        "Side conversation: read-only tools; nothing here joins the main conversation. /back to return.",
      );
    }
    for (const it of this.postReload) this.add(it);
    this.postReload = [];
    await this.refreshState();
  }

  handle(e: CoreEvent) {
    switch (e.type) {
      case "turn_start":
        this.busy = true;
        break;
      case "message_start":
        if (!this.cur || this.cur.text || this.cur.thinking || this.cur.composing) {
          this.cur = this.add({
            kind: "assistant",
            text: "",
            thinking: "",
            cancelled: false,
            streaming: true,
            composing: "",
          }) as Extract<Item, { kind: "assistant" }>;
        }
        break;
      case "text_delta":
        if (this.cur) {
          this.cur.text += e.text;
          this.cur.composing = "";
        }
        break;
      case "thinking_delta":
        if (this.cur) this.cur.thinking += e.text;
        break;
      case "tool_call_delta":
        if (this.cur && e.name) this.cur.composing = e.name;
        break;
      case "message_end":
        if (e.message.role === "user") {
          const { text, attached } = userText(e.message);
          this.add({ kind: "user", text, attached });
        } else if (e.message.role === "assistant" && this.cur) {
          this.cur.text = resultText(e.message.content);
          this.cur.streaming = false;
          this.cur.composing = "";
          this.cur.cancelled = e.message.meta?.stop_reason === "cancelled";
          this.dropIfEmpty(this.cur);
          this.cur = null;
        }
        break;
      case "tool_start": {
        const item = this.add({
          kind: "tool",
          tool: {
            id: e.id,
            name: e.name,
            args: e.args,
            running: true,
            output: "",
            result: "",
            isError: false,
            display: null,
            durationMs: 0,
          },
        });
        if (item.kind === "tool") this.tools.set(e.id, item.tool);
        break;
      }
      case "tool_output": {
        const t = this.tools.get(e.id);
        if (t) {
          t.output += e.text;
          if (t.output.length > maxToolOutput) t.output = t.output.slice(-maxToolOutput / 2);
        }
        break;
      }
      case "tool_end": {
        const t = this.tools.get(e.id);
        if (t) {
          t.running = false;
          t.isError = !!e.result.is_error;
          t.result = resultText(e.result.content);
          t.display = e.display ?? null;
          t.durationMs = e.duration_ns / 1e6;
        }
        break;
      }
      case "usage":
        this.contextTokens = e.context_tokens;
        this.contextWindow = e.context_window;
        this.cost = e.session_cost;
        break;
      case "retry":
        this.notice(
          "warn",
          `Retrying in ${(e.delay_ns / 1e9).toFixed(1)}s (attempt ${e.attempt}/${e.max}): ${e.error}`,
        );
        break;
      case "error":
        this.notice("error", e.message);
        break;
      case "notice":
        this.notice(e.level, e.text);
        break;
      case "compaction_start":
        this.compaction = this.add({
          kind: "compaction",
          running: true,
          reason: e.reason,
          before: e.tokens,
          after: 0,
          summary: "",
          error: "",
        }) as Extract<Item, { kind: "compaction" }>;
        break;
      case "compaction_end":
        if (this.compaction) {
          const c = this.compaction;
          c.running = false;
          c.after = e.tokens_after;
          c.error = e.error ?? "";
          if (e.entry_id) void Bridge.CompactionSummary(e.entry_id).then((s) => (c.summary = s));
          this.compaction = null;
        }
        break;
      case "status":
        if (e.text) this.statuses[e.key] = e.text;
        else delete this.statuses[e.key];
        break;
      case "queue":
        this.steering = e.steering ?? [];
        this.followUps = e.follow_ups ?? [];
        break;
      case "model_changed":
        this.notice("info", "Model: " + e.model);
        if (this.state) this.state.model = e.model;
        break;
      case "session_changed":
        void this.loadTranscript();
        break;
      case "turn_end":
        this.busy = false;
        if (this.cur) {
          this.cur.streaming = false;
          this.dropIfEmpty(this.cur);
          this.cur = null;
        }
        for (const t of this.tools.values()) {
          if (t.running) {
            t.running = false;
            t.isError = true;
          }
        }
        if (e.reason === "cancelled") this.notice("warn", "Cancelled");
        void Bridge.TakeQueue().then((left) => {
          if (left && left.length > 0) this.draft = left.join("\n\n");
        });
        void this.refreshState();
        break;
    }
  }

  private dropIfEmpty(a: Extract<Item, { kind: "assistant" }>) {
    if (!a.text && !a.thinking && !a.cancelled) {
      const i = this.items.indexOf(a);
      if (i >= 0) this.items.splice(i, 1);
    }
  }
}

export const store = new Store();
