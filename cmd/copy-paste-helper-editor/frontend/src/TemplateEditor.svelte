<script lang="ts">
  import type { template } from '../wailsjs/go/models'
  import ButtonRow from './ButtonRow.svelte'

  let {
    tpl = $bindable(),
    buttonKeys,
    errors,
    previews,
    validating,
    saving,
    saveMessage,
    onsave
  }: {
    tpl: template.Template
    buttonKeys: string[]
    errors: string[]
    previews: Record<string, string>
    validating: boolean
    saving: boolean
    saveMessage: string
    onsave: () => void
  } = $props()

  const usedKeys = $derived(new Set(tpl.buttons.map((b) => b.button)))
  const availableKeys = $derived(buttonKeys.filter((k) => !usedKeys.has(k)))

  function addButton() {
    const key = availableKeys[0]
    if (!key) return
    tpl.buttons = [...tpl.buttons, { button: key, id: '', template: '', toolTip: '', mode: '' }]
  }

  function removeButton(index: number) {
    tpl.buttons = tpl.buttons.filter((_, i) => i !== index)
  }
</script>

<div class="editor">
  <div class="header">
    <label class="name-field">
      <span>Template name</span>
      <input type="text" bind:value={tpl.name} placeholder="e.g. Work" />
    </label>
    {#if tpl.path}
      <p class="path" title={tpl.path}>{tpl.path}</p>
    {:else}
      <p class="path new">Not saved yet</p>
    {/if}
  </div>

  <div class="buttons">
    {#each tpl.buttons as _, i (i)}
      <ButtonRow
        bind:btn={tpl.buttons[i]}
        {buttonKeys}
        {usedKeys}
        preview={previews[tpl.buttons[i].button]}
        onremove={() => removeButton(i)}
      />
    {/each}

    {#if tpl.buttons.length === 0}
      <p class="empty-hint">No buttons yet — every key does nothing while this template is active.</p>
    {/if}
  </div>

  <button type="button" class="add-button" disabled={availableKeys.length === 0} onclick={addButton}>
    + Add button{availableKeys.length === 0 ? ' (all keys are used)' : ''}
  </button>

  {#if errors.length > 0}
    <div class="errors">
      <strong>Can't save yet:</strong>
      <ul>
        {#each errors as e (e)}
          <li>{e}</li>
        {/each}
      </ul>
    </div>
  {/if}

  <div class="actions">
    <button type="button" class="save" disabled={saving || errors.length > 0} onclick={onsave}>
      {saving ? 'Saving…' : 'Save'}
    </button>
    {#if validating}
      <span class="status validating">Checking…</span>
    {:else if saveMessage}
      <span class="status" class:ok={saveMessage === 'Saved.'}>{saveMessage}</span>
    {/if}
  </div>
</div>

<style>
  .editor {
    display: flex;
    flex-direction: column;
    gap: 16px;
    max-width: 720px;
  }

  .header {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .name-field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .name-field span {
    font-size: 0.85em;
    color: var(--text-dim);
  }

  .name-field input {
    font-size: 1.1em;
    font-weight: 600;
  }

  .path {
    margin: 0;
    font-size: 0.8em;
    color: var(--text-dim);
    font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .path.new {
    font-style: italic;
    font-family: inherit;
  }

  .buttons {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .empty-hint {
    color: var(--text-dim);
    font-style: italic;
  }

  .add-button {
    align-self: flex-start;
    background: var(--panel-alt);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 14px;
    cursor: pointer;
  }

  .add-button:hover:not(:disabled) {
    border-color: var(--accent);
  }

  .add-button:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .errors {
    background: var(--danger-dim);
    border: 1px solid var(--danger);
    border-radius: 8px;
    padding: 10px 14px;
  }

  .errors strong {
    color: var(--danger);
  }

  .errors ul {
    margin: 6px 0 0;
    padding-left: 1.2em;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .save {
    background: var(--accent);
    border: none;
    border-radius: 6px;
    padding: 8px 20px;
    font-weight: 600;
    cursor: pointer;
    color: #0b1220;
  }

  .save:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .status {
    color: var(--text-dim);
  }

  .status.ok {
    color: var(--ok);
  }

  .status.validating {
    font-style: italic;
  }
</style>
