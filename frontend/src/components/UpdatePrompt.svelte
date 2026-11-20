<script lang="ts">
  import { Bridge } from "../../bindings/github.com/NDari/rho-gui/bridge";

  // Offers a newer version: installs it and restarts the window in the
  // same folder and session.
  let {
    version,
    busy,
    onlater,
  }: {
    version: string;
    busy: boolean;
    onlater: () => void;
  } = $props();

  let phase: "offer" | "installing" | "installed" | "failed" = $state("offer");
  let notes: string[] = $state([]);
  let problem = $state("");

  function errText(err: unknown): string {
    if (err && typeof err === "object" && "message" in err) return String((err as Error).message);
    return String(err);
  }

  async function install() {
    phase = "installing";
    try {
      const res = await Bridge.InstallUpdate();
      notes = res.notes ?? [];
      phase = "installed";
      // Restart at once unless there is something to read first.
      if (notes.length === 0) await restart();
    } catch (err) {
      problem = errText(err);
      phase = "failed";
    }
  }

  async function restart() {
    try {
      await Bridge.Restart();
    } catch (err) {
      problem = errText(err);
    }
  }
</script>

<div class="update" role="status">
  {#if phase === "offer"}
    <div class="title">Rho {version} is available</div>
    <div class="buttons">
      <button onclick={onlater}>Later</button>
      <button
        class="primary"
        disabled={busy}
        title={busy ? "Wait for the agent to finish, or cancel it" : ""}
        onclick={() => void install()}>Update and restart</button
      >
    </div>
  {:else if phase === "installing"}
    <div class="title">Updating to Rho {version}…</div>
  {:else if phase === "installed"}
    <div class="title">Rho {version} is installed</div>
    {#each notes as n, i (i)}<div class="note">{n}</div>{/each}
    {#if problem}<div class="error">{problem}</div>{/if}
    <div class="buttons">
      <button onclick={onlater}>Later</button>
      <button class="primary" disabled={busy} onclick={() => void restart()}>Restart now</button>
    </div>
  {:else}
    <div class="title">The update failed</div>
    <div class="error">{problem}</div>
    <div class="buttons">
      <button onclick={onlater}>Close</button>
    </div>
  {/if}
</div>

<style>
  .update {
    position: fixed;
    top: 12px;
    right: 12px;
    width: min(380px, 92vw);
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.35);
    z-index: 15;
  }
  .title {
    font-weight: 600;
  }
  .note {
    color: var(--fg-muted);
    font-size: 12.5px;
    overflow-wrap: anywhere;
  }
  .error {
    color: var(--err);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .buttons {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
