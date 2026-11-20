<script lang="ts">
  import type { ToolState } from "../lib/types";
  import { duration, relPath } from "../lib/format";
  import { highlight, langForPath } from "../lib/markdown";
  import DiffView from "./DiffView.svelte";

  let { tool, cwd, expandAll }: { tool: ToolState; cwd: string; expandAll: boolean } = $props();

  let open: boolean | null = $state(null);
  const expanded = $derived(open ?? expandAll);

  const args = $derived(
    tool.args && typeof tool.args === "object" ? (tool.args as Record<string, unknown>) : {},
  );

  const summary = $derived.by(() => {
    const str = (k: string) => (typeof args[k] === "string" ? (args[k] as string) : "");
    switch (tool.name) {
      case "read": {
        let s = relPath(cwd, str("path"));
        if (typeof args.offset === "number" && args.offset > 0) s += ":" + args.offset;
        return s;
      }
      case "write":
      case "edit":
        return relPath(cwd, str("path"));
      case "bash": {
        const cmd = str("command");
        const i = cmd.indexOf("\n");
        return i >= 0 ? cmd.slice(0, i) + " …" : cmd;
      }
    }
    return Object.keys(args).length ? JSON.stringify(args) : "";
  });

  const stat = $derived.by(() => {
    const d = tool.display;
    if (!d) return "";
    if (d.diff) return `+${d.diff.added} -${d.diff.removed}`;
    if (tool.name === "bash" && d.exit_code != null && d.exit_code > 0) return `exit ${d.exit_code}`;
    if (tool.name === "read") return d.summary ?? "";
    return "";
  });

  const bashText = $derived((tool.display ? (tool.display.text ?? "") : tool.result).replace(/\n+$/, ""));
  const lines = (s: string) => (s ? s.split("\n") : []);
  const tail = (s: string, n: number) => lines(s).slice(-n).join("\n");

  // While running, keep the live output scrolled to the end.
  let liveEl: HTMLPreElement | undefined = $state();
  $effect(() => {
    void tool.output;
    if (liveEl) liveEl.scrollTop = liveEl.scrollHeight;
  });
</script>

<div class="tool" class:error={tool.isError}>
  <button class="head" onclick={() => (open = !expanded)} title={expanded ? "Collapse" : "Expand"}>
    <span class="icon" class:running={tool.running}>
      {#if tool.running}●{:else if tool.isError}✗{:else}✓{/if}
    </span>
    <span class="name">{tool.name}</span>
    <span class="summary">{summary}</span>
    <span class="right">
      {stat}{stat && tool.durationMs ? " · " : ""}{tool.durationMs ? duration(tool.durationMs) : ""}
    </span>
  </button>

  {#if tool.running}
    {#if tool.output}
      <pre class="out live" bind:this={liveEl}>{tool.output}</pre>
    {/if}
  {:else if tool.display?.diff}
    <div class="body">
      <DiffView diff={tool.display.diff} path={tool.display.path ?? ""} maxRows={expanded ? 0 : 40} />
    </div>
  {:else if tool.isError}
    <pre class="out err">{expanded ? tool.result : tail(tool.result, 6)}</pre>
  {:else if tool.name === "bash"}
    {#if !bashText}
      <div class="body muted">(no output)</div>
    {:else if expanded || lines(bashText).length <= 3}
      <pre class="out">{bashText}</pre>
    {:else}
      <pre class="out">{tail(bashText, 3)}</pre>
      <div class="body muted">{lines(bashText).length} lines, click the header to expand</div>
    {/if}
    {#if tool.display?.full_output_path && expanded}
      <div class="body muted">Full output: {tool.display.full_output_path}</div>
    {/if}
  {:else if expanded}
    {#if tool.name === "read" && tool.display?.text}
      <pre class="out code"><code class="hljs"
          >{@html highlight(tool.display.text, langForPath(tool.display.path ?? ""))}</code
        ></pre>
    {:else if tool.result}
      <pre class="out">{tool.result}</pre>
    {/if}
  {/if}
</div>

<style>
  .tool {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-soft);
    overflow: hidden;
  }
  .head {
    display: flex;
    gap: 8px;
    align-items: baseline;
    width: 100%;
    text-align: left;
    background: none;
    border: none;
    border-radius: 0;
    padding: 5px 10px;
    font-family: var(--mono);
    font-size: 12.5px;
  }
  .icon {
    color: var(--ok);
  }
  .icon.running {
    color: var(--accent);
    animation: pulse 1s ease-in-out infinite;
  }
  .error .icon {
    color: var(--err);
  }
  .name {
    font-weight: 600;
    color: var(--accent);
  }
  .summary {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .right {
    color: var(--fg-muted);
    white-space: nowrap;
  }
  .body {
    padding: 0 10px 8px;
  }
  .muted {
    color: var(--fg-muted);
    font-size: 12.5px;
  }
  .out {
    margin: 0;
    padding: 4px 10px 8px;
    max-height: 480px;
    overflow: auto;
    color: var(--fg-muted);
    white-space: pre-wrap;
    word-break: break-word;
  }
  .out.live {
    max-height: 160px;
  }
  .out.code {
    white-space: pre;
    color: var(--fg);
  }
  .out.err {
    color: var(--err);
  }
  @keyframes pulse {
    50% {
      opacity: 0.3;
    }
  }
</style>
