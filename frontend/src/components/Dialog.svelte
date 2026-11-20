<script lang="ts" module>
  export interface DialogSpec {
    kind: "confirm" | "input";
    title: string;
    message?: string;
    placeholder?: string;
    initial?: string;
    respond: (value: string, ok: boolean) => void;
  }
</script>

<script lang="ts">
  import { onMount } from "svelte";

  let { spec }: { spec: DialogSpec } = $props();
  // svelte-ignore state_referenced_locally
  let value = $state(spec.initial ?? "");
  let inputEl: HTMLInputElement | undefined = $state();
  let okEl: HTMLButtonElement | undefined = $state();

  onMount(() => (spec.kind === "input" ? inputEl?.focus() : okEl?.focus()));

  function key(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.preventDefault();
      spec.respond("", false);
    } else if (e.key === "Enter") {
      e.preventDefault();
      spec.respond(value, true);
    } else if (spec.kind === "confirm" && (e.key === "y" || e.key === "n")) {
      e.preventDefault();
      spec.respond("", e.key === "y");
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onkeydown={key}>
  <div class="dialog" role="dialog" aria-modal="true" aria-label={spec.title}>
    <div class="title">{spec.title}</div>
    {#if spec.message}<div class="message">{spec.message}</div>{/if}
    {#if spec.kind === "input"}
      <input bind:this={inputEl} bind:value placeholder={spec.placeholder ?? ""} />
    {/if}
    <div class="buttons">
      <button onclick={() => spec.respond("", false)}>{spec.kind === "confirm" ? "No" : "Cancel"}</button>
      <button class="primary" bind:this={okEl} onclick={() => spec.respond(value, true)}>
        {spec.kind === "confirm" ? "Yes" : "OK"}
      </button>
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.35);
    display: flex;
    justify-content: center;
    align-items: flex-start;
    padding-top: 15vh;
    z-index: 20;
  }
  .dialog {
    width: min(520px, 92vw);
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 14px 16px;
    display: flex;
    flex-direction: column;
    gap: 10px;
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.35);
  }
  .title {
    font-weight: 600;
  }
  .message {
    white-space: pre-wrap;
    color: var(--fg-muted);
  }
  .buttons {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
