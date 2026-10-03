import { mount } from 'svelte';
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import App from './App.svelte';
import './style.css';

const monacoGlobal = globalThis as typeof globalThis & {
  MonacoEnvironment?: { getWorker: () => Worker };
};
monacoGlobal.MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
};

const target = document.getElementById('app');
if (!target) throw new Error('Missing application mount point');
mount(App, { target });
