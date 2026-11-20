<script lang="ts">
  import type { Diff } from "../../bindings/github.com/NDari/rho/core/events/models";
  import { highlight, langForPath } from "../lib/markdown";

  let { diff, path, maxRows = 0 }: { diff: Diff; path: string; maxRows?: number } = $props();

  interface Row {
    sep?: boolean;
    kind?: "add" | "del" | "ctx";
    oldNo?: number;
    newNo?: number;
    html?: string;
  }

  const lang = $derived(langForPath(path));
  const rows = $derived.by(() => {
    const out: Row[] = [];
    for (const [i, h] of (diff.hunks ?? []).entries()) {
      if (i > 0) out.push({ sep: true });
      let o = h.old_start;
      let n = h.new_start;
      for (const l of h.lines ?? []) {
        const html = highlight(l.text, lang);
        if (l.kind === 43) out.push({ kind: "add", newNo: n++, html });
        else if (l.kind === 45) out.push({ kind: "del", oldNo: o++, html });
        else out.push({ kind: "ctx", oldNo: o++, newNo: n++, html });
      }
    }
    return out;
  });
  const shown = $derived(maxRows > 0 ? rows.slice(0, maxRows) : rows);
</script>

<div class="diff">
  <table>
    <tbody>
      {#each shown as r, i (i)}
        {#if r.sep}
          <tr class="sep"><td colspan="4">⋯</td></tr>
        {:else}
          <tr class={r.kind}>
            <td class="no">{r.oldNo ?? ""}</td>
            <td class="no">{r.newNo ?? ""}</td>
            <td class="sign">{r.kind === "add" ? "+" : r.kind === "del" ? "-" : " "}</td>
            <td class="text"><code class="hljs">{@html r.html}</code></td>
          </tr>
        {/if}
      {/each}
    </tbody>
  </table>
  {#if shown.length < rows.length}
    <div class="more">{rows.length - shown.length} more lines</div>
  {/if}
</div>

<style>
  .diff {
    background: var(--code-bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow-x: auto;
    font-family: var(--mono);
    font-size: 12.5px;
  }
  table {
    border-collapse: collapse;
    width: 100%;
  }
  td {
    padding: 0 6px;
    white-space: pre;
    vertical-align: top;
  }
  .no {
    color: var(--fg-faint);
    text-align: right;
    user-select: none;
    width: 1%;
  }
  .sign {
    width: 1%;
    user-select: none;
    color: var(--fg-faint);
  }
  .add {
    background: var(--add-bg);
  }
  .add .sign {
    color: var(--add-fg);
  }
  .del {
    background: var(--del-bg);
  }
  .del .sign {
    color: var(--del-fg);
  }
  .sep td {
    color: var(--fg-faint);
    text-align: center;
  }
  .more {
    padding: 2px 8px;
    color: var(--fg-muted);
    border-top: 1px solid var(--border);
  }
  code {
    font-size: inherit;
  }
</style>
