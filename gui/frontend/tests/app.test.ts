import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import '@testing-library/jest-dom/vitest';
import { idle } from '../src/ipc';

vi.mock('../src/bridge', () => ({ bridge: {
  available: () => true,
  attach: async () => ({ clientId: 'test-client', app: { name: 'FolderWatch', version: 'test', protocol: 1, heartbeatMillis: 2000, leaseMillis: 15000 }, status: { ...idle(), sequence: '1' } }),
  status: async () => ({ status: { ...idle(), sequence: '1' } }),
  start: vi.fn(async () => ({ status: { ...idle(), sequence: '2', state: 'Monitoring', sessionId: 'test-session', root: '/fixture' } })),
  stop: vi.fn(async () => ({ status: { ...idle(), sequence: '3', sessionId: 'test-session' } })),
  heartbeat: async () => ({ status: idle() }),
  detach: vi.fn(async () => ({ status: idle() })),
  on: () => () => {},
} }));

import App from '../src/App.svelte';
import { bridge } from '../src/bridge';

afterEach(cleanup);

it('renders the real shell controls, starts and stops through IPC', async () => {
  render(App);
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Idle'));
  const input = screen.getByRole('textbox', { name: 'Folder path' });
  const start = screen.getByRole('button', { name: 'Start monitoring' });
  const stop = screen.getByRole('button', { name: 'Stop session' });
  expect(start).toBeDisabled(); expect(stop).toBeDisabled();
  await fireEvent.input(input, { target: { value: '/fixture' } });
  await fireEvent.click(start);
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Monitoring'));
  expect(bridge.start).toHaveBeenCalledWith({ clientId: 'test-client', root: '/fixture' });
  expect(input).toBeDisabled(); expect(stop).toBeEnabled();
  await fireEvent.click(stop);
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Idle'));
  expect(input).toBeEnabled();
  expect(screen.getByText(/intentionally reserved for R7/)).toBeInTheDocument();
});
