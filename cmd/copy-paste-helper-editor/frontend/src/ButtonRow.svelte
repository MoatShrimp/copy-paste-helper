<script lang="ts">
  import type { template } from '../wailsjs/go/models'

  let {
    btn = $bindable(),
    buttonKeys,
    usedKeys,
    preview,
    onremove
  }: {
    btn: template.Button
    buttonKeys: string[]
    usedKeys: Set<string>
    preview?: string
    onremove: () => void
  } = $props()

  const isScratch = $derived(!btn.template)
  const showPreview = $derived(
    !isScratch && preview !== undefined && preview !== btn.template && btn.template!.includes('{{')
  )
</script>

<div class="row" class:scratch={isScratch}>
  <div class="row-head">
    <select bind:value={btn.button} title="Physical key">
      {#each buttonKeys as k (k)}
        <option value={k} disabled={k !== btn.button && usedKeys.has(k)}>{k.toUpperCase()}</option>
      {/each}
    </select>

    <input type="text" placeholder="id (optional)" bind:value={btn.id} class="id-field" />

    <select bind:value={btn.mode} class="mode-field" title="Paste mode override">
      <option value="">default mode</option>
      <option value="write">write</option>
      <option value="paste">paste</option>
    </select>

    <button type="button" class="remove" title="Remove this button" onclick={onremove}>✕</button>
  </div>

  <input type="text" placeholder="Tool-tip (shown in the info popup)" bind:value={btn.toolTip} class="tooltip-field" />

  <textarea
    placeholder="Template text — leave empty to make this a scratch buffer"
    bind:value={btn.template}
    rows="2"
  ></textarea>

  {#if isScratch}
    <p class="hint">Scratch buffer — first press copies your selection, later presses paste it back.</p>
  {:else if showPreview}
    <p class="preview"><span>Preview</span>{preview}</p>
  {/if}
</div>

<style>
  .row {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--panel);
  }

  .row.scratch {
    border-style: dashed;
  }

  .row-head {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .row-head select:first-child {
    font-weight: 600;
    min-width: 4.5em;
  }

  .id-field {
    flex: 1;
    min-width: 0;
  }

  .mode-field {
    flex: 0 0 auto;
  }

  .tooltip-field {
    width: 100%;
  }

  textarea {
    width: 100%;
  }

  .remove {
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 6px;
    width: 28px;
    height: 28px;
    line-height: 1;
    cursor: pointer;
    color: var(--text-dim);
  }

  .remove:hover {
    border-color: var(--danger);
    color: var(--danger);
  }

  .hint {
    margin: 0;
    font-size: 0.85em;
    color: var(--text-dim);
    font-style: italic;
  }

  .preview {
    margin: 0;
    font-size: 0.85em;
    color: var(--text-dim);
    white-space: pre-wrap;
    font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
  }

  .preview span {
    font-family: inherit;
    font-style: normal;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--accent);
    margin-right: 0.6em;
  }
</style>
