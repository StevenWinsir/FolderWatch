<script lang="ts">
  import { afterUpdate, onDestroy, onMount } from 'svelte';
  import * as monaco from 'monaco-editor/esm/vs/editor/editor.api.js';
  import type { DiffResult } from './ipc';

  export let diff: DiffResult | undefined;
  export let loading = false;

  let container: HTMLDivElement;
  let editor: monaco.editor.IStandaloneDiffEditor | undefined;
  let original: monaco.editor.ITextModel | undefined;
  let modified: monaco.editor.ITextModel | undefined;
  let mounted = false;
  let modelKey = '';

  function sideText(side: 'original' | 'modified', currentDiff = diff) {
    if (!currentDiff) return '';
    const kinds = side === 'original' ? new Set(['context', 'removed']) : new Set(['context', 'added']);
    return currentDiff.hunks.flatMap(hunk => hunk.lines.filter(line => kinds.has(line.kind)).map(line => line.text)).join('\n');
  }

  function disposeModels() {
    editor?.setModel(null);
    original?.dispose();
    modified?.dispose();
    original = undefined;
    modified = undefined;
    modelKey = '';
  }

  function updateModels(currentDiff = diff) {
    if (!mounted) return;
    if (!currentDiff || currentDiff.status !== 'text') {
      if (modelKey) disposeModels();
      return;
    }
    ensureEditor();
    if (!editor) return;
    const nextKey = `${currentDiff.path}:${currentDiff.generation}:${currentDiff.version}`;
    if (nextKey === modelKey) return;
    disposeModels();
    modelKey = nextKey;
    original = monaco.editor.createModel(sideText('original', currentDiff), 'plaintext');
    modified = monaco.editor.createModel(sideText('modified', currentDiff), 'plaintext');
    editor.setModel({ original, modified });
  }

  function ensureEditor() {
    if (editor || !container || typeof window.matchMedia !== 'function') return;
    editor = monaco.editor.createDiffEditor(container, {
      automaticLayout: true,
      readOnly: true,
      originalEditable: false,
      renderSideBySide: true,
      minimap: { enabled: false },
      folding: false,
      lineNumbers: 'on',
      glyphMargin: false,
      renderOverviewRuler: false,
      scrollBeyondLastLine: false,
      padding: { top: 14, bottom: 14 },
      fontSize: 12,
      lineHeight: 20,
      wordWrap: 'on',
      theme: 'vs-dark',
    });
  }

  onMount(() => {
    mounted = true;
    // Lightweight component tests do not provide Monaco's browser APIs.
    updateModels(diff);
  });

  // Apply asynchronous diff responses after DOM bindings and visibility update.
  afterUpdate(() => {
    if (mounted) updateModels(diff);
  });

  onDestroy(() => {
    disposeModels();
    editor?.dispose();
  });
</script>

<div class="diff-shell">
  <!-- Preserve the host across loading/empty/binary states: Monaco owns its DOM. -->
  <div bind:this={container} class="monaco-host" class:hidden={loading || diff?.status !== 'text'} aria-label="Read-only file diff"></div>
  {#if loading}
    <div class="diff-state"><span class="spinner" aria-hidden="true"></span><strong>Reading diff</strong><span>Fetching file content from the local core…</span></div>
  {:else if !diff}
    <div class="diff-state"><div class="state-mark">⌁</div><strong>Select a changed file</strong><span>Choose a file on the left to inspect its before and after content.</span></div>
  {:else if diff.status !== 'text'}
    <div class="diff-state"><div class="state-mark muted">{diff.status === 'binary' ? '◈' : '⊘'}</div><strong>{diff.status === 'binary' ? 'Binary file' : diff.status === 'too-large' ? 'File is too large to preview' : diff.status === 'unsupported-text' ? 'Text encoding is not supported' : 'Preview unavailable'}</strong><span>{diff.reason || 'FolderWatch keeps this file metadata-only.'}</span></div>
  {/if}
</div>
