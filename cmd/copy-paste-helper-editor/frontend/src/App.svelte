<script lang="ts">
  import { onMount } from 'svelte'
  import { template } from '../wailsjs/go/models'
  import { ListTemplates, SaveTemplate, DeleteTemplate, ValidateTemplate, PreviewText, ButtonKeys } from '../wailsjs/go/main/App'
  import TemplateEditor from './TemplateEditor.svelte'

  let templates = $state<template.Template[]>([])
  let buttonKeys = $state<string[]>([])
  let selected = $state<template.Template | null>(null)
  let errors = $state<string[]>([])
  let previews = $state<Record<string, string>>({})
  let validating = $state(false)
  let saving = $state(false)
  let saveMessage = $state('')
  let loadError = $state('')

  function clone<T>(v: T): T {
    return JSON.parse(JSON.stringify(v))
  }

  onMount(async () => {
    try {
      buttonKeys = await ButtonKeys()
      await reload()
    } catch (e) {
      loadError = String(e)
    }
  })

  async function reload() {
    templates = await ListTemplates()
    if (selected) {
      const match = templates.find((t) => t.path === selected!.path)
      if (match) {
        select(match)
        return
      }
    }
    if (templates.length > 0) {
      select(templates[0])
    } else {
      selected = null
    }
  }

  function select(t: template.Template) {
    selected = clone(t)
    saveMessage = ''
  }

  function newTemplate() {
    selected = new template.Template({ name: 'New template', path: '', buttons: [] })
    saveMessage = ''
  }

  let validateTimer: ReturnType<typeof setTimeout>
  $effect(() => {
    if (!selected) return
    JSON.stringify(selected) // read every nested field, so this effect reruns on any edit
    clearTimeout(validateTimer)
    validateTimer = setTimeout(runValidation, 350)
  })

  async function runValidation() {
    if (!selected) return
    const snapshot = clone(selected)
    validating = true
    const errs = await ValidateTemplate(snapshot)
    if (JSON.stringify(clone(selected)) !== JSON.stringify(snapshot)) return // stale response
    errors = errs
    validating = false

    const nextPreviews: Record<string, string> = {}
    if (errs.length === 0) {
      for (const b of snapshot.buttons) {
        if (b.template) nextPreviews[b.button] = await PreviewText(snapshot, b.template)
      }
    }
    previews = nextPreviews
  }

  async function save() {
    if (!selected) return
    saving = true
    saveMessage = ''
    try {
      const result = await SaveTemplate(clone(selected))
      if (result.errors && result.errors.length > 0) {
        errors = result.errors
        saveMessage = "Not saved — fix the errors below."
        return
      }
      saveMessage = 'Saved.'
      selected = result.template!
      await reload()
    } catch (e) {
      saveMessage = `Not saved — ${e}`
    } finally {
      saving = false
    }
  }

  async function remove(t: template.Template) {
    if (!confirm(`Delete "${t.name}"? This can't be undone.`)) return
    await DeleteTemplate(t.path)
    if (selected?.path === t.path) selected = null
    await reload()
  }

  function onKeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault()
      if (selected && !saving && errors.length === 0) save()
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="layout">
  <aside>
    <h1>Templates</h1>

    {#if loadError}
      <p class="load-error">{loadError}</p>
    {/if}

    <ul>
      {#each templates as t (t.path)}
        <li class:active={selected?.path === t.path}>
          <button type="button" class="template-name" onclick={() => select(t)}>{t.name}</button>
          <button type="button" class="delete" title="Delete {t.name}" onclick={() => remove(t)}>✕</button>
        </li>
      {/each}
    </ul>

    <button type="button" class="new" onclick={newTemplate}>+ New Template</button>
  </aside>

  <main>
    {#if selected}
      {#key selected.path}
        <TemplateEditor bind:tpl={selected} {buttonKeys} {errors} {previews} {validating} {saving} {saveMessage} onsave={save} />
      {/key}
    {:else}
      <div class="empty">Select a template on the left, or create a new one.</div>
    {/if}
  </main>
</div>

<style>
  .layout {
    display: flex;
    height: 100vh;
  }

  aside {
    width: 220px;
    flex: none;
    background: var(--panel);
    border-right: 1px solid var(--border);
    padding: 16px 12px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  h1 {
    font-size: 1em;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-dim);
    margin: 0 4px;
  }

  .load-error {
    color: var(--danger);
    font-size: 0.85em;
    margin: 0 4px;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    overflow-y: auto;
  }

  li {
    display: flex;
    align-items: center;
    border-radius: 6px;
  }

  li.active {
    background: var(--accent-dim);
  }

  .template-name {
    flex: 1;
    text-align: left;
    background: transparent;
    border: none;
    padding: 8px 10px;
    cursor: pointer;
    border-radius: 6px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  li:not(.active) .template-name:hover {
    background: var(--panel-alt);
  }

  .delete {
    background: transparent;
    border: none;
    color: var(--text-dim);
    padding: 8px 10px;
    cursor: pointer;
    visibility: hidden;
  }

  li:hover .delete {
    visibility: visible;
  }

  .delete:hover {
    color: var(--danger);
  }

  .new {
    margin-top: auto;
    background: transparent;
    border: 1px dashed var(--border);
    border-radius: 6px;
    padding: 8px;
    cursor: pointer;
    color: var(--text-dim);
  }

  .new:hover {
    border-color: var(--accent);
    color: var(--text);
  }

  main {
    flex: 1;
    padding: 24px 28px;
    overflow-y: auto;
  }

  .empty {
    color: var(--text-dim);
    font-style: italic;
  }
</style>
