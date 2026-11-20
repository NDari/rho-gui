<script lang="ts">
  import { renderMarkdown, splitBlocks } from "../lib/markdown";

  // While streaming, finished blocks keep their DOM (and any text
  // selection) and only the last block is rendered again.
  let { src, streaming = false }: { src: string; streaming?: boolean } = $props();
  const blocks = $derived(streaming ? splitBlocks(src) : [src]);
</script>

<div class="markdown">
  {#each blocks as block, i (i)}{@html renderMarkdown(block)}{/each}
</div>
