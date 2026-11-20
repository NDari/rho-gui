<script lang="ts">
  import { humanTokens, shortenHome } from "../lib/format";

  let {
    model,
    cwd,
    branch,
    contextTokens,
    contextWindow,
    cost,
    aside,
    busy,
    statuses,
    onmodel,
  }: {
    model: string;
    cwd: string;
    branch: string;
    contextTokens: number;
    contextWindow: number;
    cost: number;
    aside: boolean;
    busy: boolean;
    statuses: Record<string, string>;
    onmodel: () => void;
  } = $props();

  const pct = $derived(contextWindow > 0 ? Math.round((contextTokens * 100) / contextWindow) : 0);
</script>

<div class="status">
  <button class="model" onclick={onmodel} title="Switch model (/model)">{model}</button>
  <span title="Context used">
    {#if contextWindow > 0}
      {pct}% · {humanTokens(contextTokens)}/{humanTokens(contextWindow)}
    {:else}
      {humanTokens(contextTokens)} tokens
    {/if}
  </span>
  {#if cost > 0}<span title="Session cost">${cost.toFixed(cost < 1 ? 3 : 2)}</span>{/if}
  {#if aside}<span class="aside">side conversation</span>{/if}
  {#each Object.entries(statuses) as [k, v] (k)}<span>{v}</span>{/each}
  <span class="spacer"></span>
  {#if busy}<span class="busy">working</span>{/if}
  <span class="cwd" title={cwd}>{shortenHome(cwd)}{branch ? ` (${branch})` : ""}</span>
</div>

<style>
  .status {
    display: flex;
    gap: 14px;
    align-items: center;
    padding: 4px 14px;
    font-size: 12px;
    color: var(--fg-muted);
    border-top: 1px solid var(--border);
    background: var(--bg-soft);
    white-space: nowrap;
    overflow: hidden;
  }
  .model {
    border: none;
    background: none;
    padding: 0;
    color: var(--accent);
    font-weight: 600;
  }
  .aside {
    color: var(--warn);
  }
  .busy {
    color: var(--accent);
    animation: pulse 1.2s ease-in-out infinite;
  }
  .spacer {
    flex: 1;
  }
  .cwd {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  @keyframes pulse {
    50% {
      opacity: 0.4;
    }
  }
</style>
