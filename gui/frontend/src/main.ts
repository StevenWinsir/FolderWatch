import { mount } from 'svelte';
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import App from './App.svelte';
import { shouldMountFrontend } from './frontend-mode';
import './style.css';

const monacoGlobal = globalThis as typeof globalThis & {
  MonacoEnvironment?: { getWorker: () => Worker };
};
monacoGlobal.MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
};

const target = document.getElementById('app');
if (!target) throw new Error('Missing application mount point');
if (shouldMountFrontend(import.meta.env.DEV, import.meta.env.VITE_FW_E2E_BROWSER_ONLY, location.protocol)) {
  mount(App, { target });
} else {
  target.textContent = 'Browser IPC test mode. The test browser owns the frontend connection.';
}
