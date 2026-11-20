<script lang="ts">
  import { Clipboard } from "@wailsio/runtime";
  import type { StartupInfo } from "../../bindings/github.com/NDari/rho-gui/bridge";
  import { shortenHome } from "../lib/format";

  // Shown instead of the transcript when the project could not be opened,
  // for example because no model is configured yet.
  let {
    info,
    onretry,
    onopen,
    onconfig,
    oncreate,
  }: {
    info: StartupInfo;
    onretry: () => Promise<string>;
    onopen: () => Promise<string>;
    onconfig: () => Promise<string>;
    oncreate: () => Promise<string>;
  } = $props();

  const example = $derived(info.example.trimEnd());

  let problem = $state("");
  let copied = $state(false);

  async function run(f: () => Promise<string>) {
    problem = await f();
  }
</script>

<div class="setup">
  <div class="logo">ρ</div>
  <h1>Rho could not start</h1>
  <pre class="error">{info.error}</pre>
  <p>
    Rho was opening <b>{shortenHome(info.dir)}</b>. If no model is configured yet, add one to
    <code>{shortenHome(info.config_path)}</code>, for example for a local OpenAI-compatible server such as
    llama.cpp's <code>llama-server</code> on its default port 9931 (older versions use 8080):
  </p>
  <div class="example">
    <pre>{example}</pre>
    <button
      class="copy"
      onclick={async () => {
        copied = await Clipboard.SetText(example).then(
          () => true,
          () => false,
        );
      }}>{copied ? "Copied" : "Copy"}</button
    >
  </div>
  <p class="muted">
    A hosted provider works too: set its API key (for example <code>OPENAI_API_KEY</code>) and start Rho with
    <code>-m openai/&lt;model-name&gt;</code>. See the configuration guide in the Rho documentation for all settings.
  </p>
  <div class="actions">
    {#if info.config_exists}
      <button class="primary" onclick={() => void run(onretry)}>Try again</button>
      <button onclick={() => void run(onconfig)}>Open the config file</button>
    {:else}
      <button class="primary" onclick={() => void run(oncreate)}>Create the config file with this example</button>
      <button onclick={() => void run(onretry)}>Try again</button>
    {/if}
    <button onclick={() => void run(onopen)}>Open another folder…</button>
  </div>
  {#if problem}
    <pre class="error">{problem}</pre>
  {/if}
</div>

<style>
  .setup {
    max-width: 680px;
    margin: 8vh auto;
    padding: 0 24px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .logo {
    font-size: 48px;
    color: var(--accent);
    line-height: 1;
  }
  h1 {
    font-size: 20px;
    margin: 0;
  }
  p {
    margin: 0;
    line-height: 1.5;
  }
  pre {
    margin: 0;
    font-family: var(--mono);
    white-space: pre-wrap;
  }
  .error {
    color: var(--err);
    background: var(--bg-soft);
    border-radius: 8px;
    padding: 10px 12px;
  }
  .example {
    position: relative;
    background: var(--bg-soft);
    border-radius: 8px;
    padding: 10px 12px;
  }
  .copy {
    position: absolute;
    top: 8px;
    right: 8px;
  }
  .muted {
    color: var(--fg-muted);
  }
  .actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
</style>
