<script lang="ts">
  import { onMount } from 'svelte';
  import { bridge } from './bridge';
  import { SessionController } from './controller';

  const controller = new SessionController(bridge);
  const state = controller.state;
  let root = '';
  $: active = ['Scanning', 'Monitoring', 'Paused', 'Stopping'].includes($state.status.state);
  $: canStart = $state.connected && !active && !$state.pendingStart && root.length > 0;
  $: canStop = $state.connected && ['Scanning', 'Monitoring', 'Paused'].includes($state.status.state);

  onMount(() => {
    void controller.connect();
    const pagehide = () => controller.dispose();
    window.addEventListener('pagehide', pagehide);
    return () => { window.removeEventListener('pagehide', pagehide); controller.dispose(); };
  });
</script>

<svelte:head><title>FolderWatch</title></svelte:head>
<main>
  <header class="toolbar">
    <div class="brand"><svg aria-hidden="true" viewBox="0 0 32 32"><path d="M4 9a3 3 0 0 1 3-3h7l3 4h8a3 3 0 0 1 3 3v11a3 3 0 0 1-3 3H7a3 3 0 0 1-3-3Z"/><path d="m11 18 3 3 7-7"/></svg><strong>FolderWatch</strong></div>
    <span class="local">Local files. Local processing.</span>
  </header>
  <section class="workspace" aria-labelledby="heading">
    <div class="intro">
      <h1 id="heading">Keep an eye on your folder.</h1>
      <p>Start a monitoring session. FolderWatch compares changes with the folder’s state at startup, using the same Go core as the terminal app.</p>
    </div>
    <form on:submit|preventDefault={() => controller.start(root)}>
      <label for="root">Folder path</label>
      <div class="path-row"><input id="root" bind:value={root} disabled={active || $state.pendingStart} placeholder="/Users/you/Projects/my-project" spellcheck="false" autocomplete="off" aria-describedby="path-help" /><button class="primary" type="submit" disabled={!canStart}>Start monitoring</button></div>
      <p class="hint" id="path-help">Use an absolute path or ~/path. Folder selection and the full diff workspace arrive in R7.</p>
    </form>
    <section class="session" aria-labelledby="session-heading">
      <div class="session-header"><h2 id="session-heading">Session</h2><span class:monitoring={$state.status.state === 'Monitoring'} class="status" role="status">{$state.connected ? $state.status.state : $state.connecting ? 'Connecting' : 'Disconnected'}</span></div>
      {#if active}
        <p class="session-root" title={$state.status.root}>{$state.status.root}</p>
        <p>{$state.status.state === 'Scanning' ? 'Building the startup baseline. You can stop this scan at any time.' : $state.status.state === 'Stopping' ? 'Stopping the watcher and cleaning up this session.' : 'Watching for changes. File content is requested only when a diff is needed.'}</p>
      {:else}
        <p class="empty-title">{$state.status.state === 'Error' ? 'This session ended with an error.' : 'No folder is being monitored.'}</p>
        <p>Your files are never modified. Stopping or reloading closes the session and clears its temporary snapshots.</p>
      {/if}
      <div class="session-actions"><button type="button" on:click={() => controller.stop()} disabled={!canStop}>Stop session</button>{#if !$state.connected && !$state.connecting}<button type="button" on:click={() => controller.connect()}>Reconnect</button>{/if}</div>
      {#if $state.status.warning}<p class="notice" role="status">{$state.status.warning}</p>{/if}
      {#if $state.status.problem}<p class="error" role="alert">{$state.status.problem.message}</p>{/if}
      {#if $state.error}<p class="error" role="alert">{$state.error}</p>{/if}
    </section>
    <aside class="scope"><h2>A foundation for the desktop workspace</h2><p>This R6 shell verifies session control and the versioned IPC contract. The file list, native folder picker and Monaco diff view are intentionally reserved for R7.</p></aside>
  </section>
  <footer><span>FolderWatch {$state.app?.version ?? 'dev'} · IPC {$state.app?.protocol ?? '—'}</span><span>Baseline {$state.status.generation} · Changes version {$state.status.version}</span></footer>
</main>
