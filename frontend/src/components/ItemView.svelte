<script lang="ts">
  import type { Item } from "../lib/types";
  import { humanTokens } from "../lib/format";
  import Markdown from "./Markdown.svelte";
  import ToolCard from "./ToolCard.svelte";

  let {
    item,
    cwd,
    showThinking,
    expandAll,
  }: { item: Item; cwd: string; showThinking: boolean; expandAll: boolean } = $props();

  let thinkingOpen: boolean | null = $state(null);
</script>

{#if item.kind === "user"}
  <div class="user">
    <div class="text">{item.text}</div>
    {#if item.attached.length}
      <div class="attached">Attached: {item.attached.join(", ")}</div>
    {/if}
  </div>
{:else if item.kind === "assistant"}
  <div class="assistant">
    {#if item.thinking}
      <details class="thinking" open={thinkingOpen ?? (showThinking || (item.streaming && !item.text))}>
        <summary
          onclick={(e) => {
            e.preventDefault();
            thinkingOpen = !(thinkingOpen ?? (showThinking || (item.streaming && !item.text)));
          }}>Thinking</summary
        >
        <div class="thinking-text">{item.thinking}</div>
      </details>
    {/if}
    {#if item.text}
      <Markdown src={item.text} streaming={item.streaming} />
    {/if}
    {#if item.composing}
      <div class="muted">Preparing {item.composing}…</div>
    {:else if item.streaming && !item.text && !item.thinking}
      <div class="muted pulse">Working…</div>
    {/if}
    {#if item.cancelled}
      <div class="muted">(cancelled)</div>
    {/if}
  </div>
{:else if item.kind === "tool"}
  <ToolCard tool={item.tool} {cwd} {expandAll} />
{:else if item.kind === "compaction"}
  <details class="compaction">
    <summary>
      {#if item.running}
        Compacting the conversation ({humanTokens(item.before)} tokens)…
      {:else if item.error}
        Compaction failed: {item.error}
      {:else}
        Compacted the conversation{item.before ? ` from ${humanTokens(item.before)} tokens` : ""}{item.after
          ? ` to ${humanTokens(item.after)}`
          : ""}
      {/if}
    </summary>
    {#if item.summary}
      <Markdown src={item.summary} />
    {/if}
  </details>
{:else if item.kind === "notice"}
  <div class="notice {item.level}">{item.text}</div>
{:else if item.kind === "text"}
  <div class="textitem">
    <div class="title">{item.title}</div>
    <Markdown src={item.body} />
  </div>
{/if}

<style>
  .user {
    align-self: flex-end;
    max-width: 85%;
    background: var(--user-bg);
    border-radius: 12px;
    padding: 8px 12px;
  }
  .user .text {
    white-space: pre-wrap;
    word-break: break-word;
  }
  .attached,
  .muted {
    color: var(--fg-muted);
    font-size: 12.5px;
  }
  .assistant {
    padding: 0 2px;
  }
  .thinking {
    color: var(--fg-muted);
    margin-bottom: 6px;
  }
  .thinking summary,
  .compaction summary {
    cursor: pointer;
    font-size: 12.5px;
    user-select: none;
  }
  .thinking-text {
    white-space: pre-wrap;
    font-style: italic;
    border-left: 2px solid var(--border);
    padding-left: 10px;
    margin-top: 4px;
  }
  .compaction {
    border: 1px dashed var(--border);
    border-radius: var(--radius);
    padding: 6px 10px;
    color: var(--fg-muted);
  }
  .notice {
    font-size: 13px;
    padding: 4px 10px;
    border-left: 3px solid var(--info);
    color: var(--fg-muted);
    white-space: pre-wrap;
  }
  .notice.warn {
    border-color: var(--warn);
  }
  .notice.error {
    border-color: var(--err);
    color: var(--err);
  }
  .notice.banner {
    border: 1px solid var(--warn);
    border-radius: var(--radius);
    color: var(--warn);
    font-weight: 600;
  }
  .textitem {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 10px 14px;
    background: var(--bg-soft);
  }
  .title {
    font-weight: 600;
    margin-bottom: 6px;
    color: var(--accent);
  }
  .pulse {
    animation: pulse 1.2s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.4;
    }
  }
</style>
