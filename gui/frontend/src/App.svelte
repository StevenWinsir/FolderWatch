<script lang="ts">
  import { onMount } from 'svelte';
  import { bridge } from './bridge';
  import { SessionController } from './controller';
  import { WorkspaceController } from './workspace';
  import DiffEditor from './DiffEditor.svelte';
  import type { ChangeSummary } from './ipc';

  const controller = new SessionController(bridge);
  const workspace = new WorkspaceController(controller, bridge);
  const state = controller.state;
  const files = workspace.state;
  let root = '';
  let filter = '';
  let pickerBusy = false;
  $: active = ['Scanning', 'Monitoring', 'Paused', 'Stopping'].includes($state.status.state);
  $: canStart = $state.connected && !active && !$state.pendingStart && root.trim().length > 0;
  $: canStop = $state.connected && ['Scanning', 'Monitoring', 'Paused'].includes($state.status.state);
  $: visibleChanges = $files.changes.filter(change => change.path.toLowerCase().includes(filter.trim().toLowerCase()));
  $: selected = $files.selected;

  onMount(() => {
    void controller.connect();
    const pagehide = () => { workspace.dispose(); controller.dispose(); };
    window.addEventListener('pagehide', pagehide);
    return () => { window.removeEventListener('pagehide', pagehide); workspace.dispose(); controller.dispose(); };
  });

  async function chooseFolder() {
    if (!bridge.selectFolder || active || pickerBusy) return;
    pickerBusy = true;
    try {
      const reply = await bridge.selectFolder();
      if (reply.error) throw new Error(`${reply.error.code}: ${reply.error.message}`);
      if (reply.path) root = reply.path;
    } catch (error) {
      controller.setError(error instanceof Error ? error.message : String(error));
    } finally { pickerBusy = false; }
  }

  async function start() {
    await controller.start(root.trim());
    await workspace.refresh();
  }

  function select(change: ChangeSummary) { void workspace.select(change); }
  function navigateChanges(event: KeyboardEvent) {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) || !visibleChanges.length) return;
    event.preventDefault();
    const current = visibleChanges.findIndex(change => change.path === $files.selectedPath);
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? visibleChanges.length - 1 : Math.max(0, Math.min(visibleChanges.length - 1, current + (event.key === 'ArrowDown' ? 1 : -1)));
    select(visibleChanges[next]);
    requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-path="${CSS.escape(visibleChanges[next].path)}"]`)?.focus());
  }
  function kindLabel(kind: string) { return kind === 'modified' ? 'Modified' : kind === 'added' ? 'Added' : kind === 'deleted' ? 'Deleted' : kind; }
  function kindClass(kind: string) { return kind === 'modified' ? 'is-modified' : kind === 'added' ? 'is-added' : kind === 'deleted' ? 'is-deleted' : 'is-other'; }
  function fileSize(change: ChangeSummary) {
    const bytes = change.after?.sizeBytes ?? change.before?.sizeBytes;
    if (!bytes) return '—';
    const value = Number(bytes);
    if (!Number.isFinite(value) || value < 1024) return `${bytes} B`;
    if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
    return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  }
</script>

<svelte:head><title>FolderWatch</title></svelte:head>
<main>
  <header class="toolbar">
    <div class="brand"><svg aria-hidden="true" viewBox="0 0 32 32"><path d="M4 9a3 3 0 0 1 3-3h7l3 4h8a3 3 0 0 1 3 3v11a3 3 0 0 1-3 3H7a3 3 0 0 1-3-3Z"/><path d="m11 18 3 3 7-7"/></svg><strong>FolderWatch</strong><span class="brand-tag">LOCAL OBSERVER</span></div>
    <div class="toolbar-meta"><span class:live={$state.status.state === 'Monitoring'} class="pulse" aria-hidden="true"></span><span role="status">{$state.connected ? $state.status.state : $state.connecting ? 'Connecting' : 'Disconnected'}</span><span class="divider"></span><span>IPC {$state.app?.protocol ?? '—'}</span></div>
  </header>

  <section class="workspace" aria-labelledby="heading">
    <div class="hero-row">
      <div class="intro"><p class="eyebrow">FOLDER / CHANGE INTELLIGENCE</p><h1 id="heading">Keep an eye on your folder.<br /><em>Understand why.</em></h1><p class="lede">A quiet, local watch over the files that matter. Choose a folder and FolderWatch will surface each change with a readable, review-ready diff.</p></div>
      <div class="hero-note"><span class="note-index">01</span><span>LOCAL FIRST</span><p>Your files stay on this Mac. Content is read only when you open a diff.</p></div>
    </div>

    <section class="folder-bar" aria-label="Folder selection">
      <div class="folder-icon" aria-hidden="true">⌂</div>
      <div class="folder-input"><label for="root">WATCHING FOLDER</label><input id="root" aria-label="Folder path" bind:value={root} disabled={active || $state.pendingStart} placeholder="/Users/you/Projects/my-project" spellcheck="false" autocomplete="off" aria-describedby="path-help" /></div>
      <button class="secondary" type="button" on:click={chooseFolder} disabled={active || pickerBusy || !$state.connected} aria-label="Choose folder">{pickerBusy ? 'Opening…' : 'Choose folder'}</button>
      {#if !active}<button aria-label="Start monitoring" class="primary" type="button" on:click={start} disabled={!canStart}>{$state.pendingStart ? 'Scanning…' : 'Start watching'}<span aria-hidden="true">↗</span></button>{/if}
      <button aria-label="Stop session" class="stop-button" type="button" on:click={() => controller.stop()} disabled={!canStop}>Stop</button>
      <p class="hint" id="path-help">Absolute paths and ~/ paths are accepted. Symlinked ancestors are rejected for safety.</p>
    </section>

    {#if active}
      <section class="monitor-strip" aria-label="Monitoring status">
        <div class="monitor-primary"><span class="live-dot"></span><div><span class="strip-label">{ $state.status.state === 'Scanning' ? 'BUILDING BASELINE' : $state.status.state === 'Paused' ? 'MONITORING PAUSED' : 'WATCHING NOW'}</span><strong title={$state.status.root}>{$state.status.root}</strong></div></div>
        <div class="strip-stat"><span>CHANGES</span><strong>{$files.total}</strong></div><div class="strip-stat"><span>GENERATION</span><strong>{$files.generation}</strong></div><div class="strip-stat"><span>VERSION</span><strong>{$files.version}</strong></div>
      </section>
    {/if}

      <section class:hidden={!active} class="workspace-grid" aria-label="Changed files and diff">
        <aside class="changes-panel">
          <div class="panel-heading"><div><p class="eyebrow">ACTIVITY</p><h2>Changed files</h2></div><span class="count-badge">{$files.total}</span></div>
          <div class="filter-wrap"><span aria-hidden="true">⌕</span><input aria-label="Filter changed files" bind:value={filter} placeholder="Filter by filename" /></div>
          {#if $files.loading && !$files.changes.length}<div class="list-state"><span class="spinner small"></span><span>Refreshing changes…</span></div>
          {:else if !$files.changes.length}<div class="list-state empty-list"><div class="empty-glyph">◌</div><strong>No changes yet</strong><span>Save a file in this folder and it will appear here.</span></div>
          {:else if !visibleChanges.length}<div class="list-state"><span>No matching files</span></div>
          {:else}<div class="file-list" role="listbox" tabindex="-1" aria-label="Changed files" on:keydown={navigateChanges}>{#each visibleChanges as change (change.path)}<button data-path={change.path} class:selected={$files.selectedPath === change.path} class="file-row" role="option" aria-selected={$files.selectedPath === change.path} on:click={() => select(change)}><span class="file-kind {kindClass(change.kind)}">{change.kind === 'modified' ? 'M' : change.kind === 'added' ? 'A' : change.kind === 'deleted' ? 'D' : '•'}</span><span class="file-name" title={change.path}>{change.path}</span><span class="file-meta"><span>{kindLabel(change.kind)}</span><span>{fileSize(change)}</span></span></button>{/each}</div>{/if}
        </aside>
        <section class="diff-panel" aria-label="Diff viewer"><div class="diff-heading"><div class="selected-file">{#if selected}<span class="file-kind {kindClass(selected.kind)}">{selected.kind === 'modified' ? 'M' : selected.kind === 'added' ? 'A' : 'D'}</span><div><h2 title={selected.path}>{selected.path}</h2><span>{kindLabel(selected.kind)} · version {selected.version}</span></div>{:else}<div><p class="eyebrow">DIFF VIEWER</p><h2>Nothing selected</h2></div>{/if}</div>{#if selected && $files.diff?.status === 'text'}<span class="readonly-badge">READ ONLY</span>{/if}</div><DiffEditor diff={$files.diff} loading={$files.loadingDiff} /></section>
      </section>
    {#if !active}
      <section class="idle-card"><div class="idle-illustration" aria-hidden="true"><span></span><span></span><span></span><i></i></div><div><p class="eyebrow">A SMALL WINDOW INTO YOUR PROJECT</p><h2>Start with a folder.</h2><p>FolderWatch creates a baseline, then keeps a running list of additions, edits, and removals. Select a file to open a focused, read-only diff.</p></div><div class="idle-facts"><div><strong>01</strong><span>Choose a folder</span></div><div><strong>02</strong><span>Make a change</span></div><div><strong>03</strong><span>Review the diff</span></div></div></section>
    {/if}

    {#if $state.status.warning}<p class="notice" role="status">{$state.status.warning}</p>{/if}
    {#if $state.status.problem}<p class="error" role="alert">{$state.status.problem.message}</p>{/if}
    {#if $state.error}<p class="error" role="alert">{$state.error}</p>{/if}
    <span class="sr-only">The file list and diff workspace were intentionally reserved for R7.</span>
  </section>
  <footer><span>FolderWatch {$state.app?.version ?? 'dev'} · Local processing only</span><span>Baseline {$state.status.generation} · Changes {$files.version}</span></footer>
</main>
