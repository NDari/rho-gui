<script lang="ts">
  import { renderMarkdown } from "../lib/markdown";

  let { src }: { src: string } = $props();
  const html = $derived(renderMarkdown(src));
</script>

<div class="markdown">{@html html}</div>
