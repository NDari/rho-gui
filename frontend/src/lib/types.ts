// Core events as sent by the bridge (see core/events and events.Marshal).

import type { Display } from "../../bindings/github.com/NDari/rho/core/events/models";

export type { Display };

export interface Block {
  type: string;
  text?: string;
  name?: string;
  content?: Block[];
}

export interface Message {
  role: "user" | "assistant" | "tool_result";
  content?: Block[];
  meta?: { stop_reason?: string; synthetic?: string };
}

export type CoreEvent =
  | { type: "turn_start" }
  | { type: "turn_end"; reason: "done" | "cancelled" | "error" }
  | { type: "message_start"; model: string }
  | { type: "text_delta"; text: string }
  | { type: "thinking_delta"; text: string }
  | { type: "tool_call_delta"; index: number; name?: string }
  | { type: "message_end"; entry_id: string; message: Message }
  | { type: "tool_start"; id: string; name: string; args: unknown }
  | { type: "tool_output"; id: string; text: string }
  | {
      type: "tool_end";
      id: string;
      name: string;
      result: { content?: { type: string; text?: string }[]; is_error?: boolean };
      display?: Display | null;
      duration_ns: number;
    }
  | {
      type: "usage";
      context_tokens: number;
      context_window: number;
      session_cost: number;
    }
  | { type: "retry"; attempt: number; max: number; delay_ns: number; error: string }
  | { type: "error"; message: string }
  | { type: "compaction_start"; reason: string; tokens: number }
  | {
      type: "compaction_end";
      entry_id?: string;
      tokens_before: number;
      tokens_after: number;
      error?: string;
    }
  | { type: "notice"; level: "info" | "warn" | "error"; text: string }
  | { type: "status"; key: string; text: string }
  | { type: "queue"; steering: string[] | null; follow_ups: string[] | null }
  | { type: "model_changed"; model: string }
  | { type: "session_changed"; session_id: string; path: string; aside: boolean };

export interface ToolState {
  id: string;
  name: string;
  args: unknown;
  running: boolean;
  output: string;
  result: string;
  isError: boolean;
  display: Display | null;
  durationMs: number;
}

export type Item =
  | { key: number; kind: "user"; text: string; attached: string[] }
  | {
      key: number;
      kind: "assistant";
      text: string;
      thinking: string;
      cancelled: boolean;
      streaming: boolean;
      composing: string;
    }
  | { key: number; kind: "tool"; tool: ToolState }
  | {
      key: number;
      kind: "compaction";
      running: boolean;
      reason: string;
      before: number;
      after: number;
      summary: string;
      error: string;
    }
  | { key: number; kind: "notice"; level: string; text: string }
  | { key: number; kind: "text"; title: string; body: string };

// Distributive Omit keeps the union members apart.
export type NewItem = Item extends infer T ? (T extends Item ? Omit<T, "key"> : never) : never;
