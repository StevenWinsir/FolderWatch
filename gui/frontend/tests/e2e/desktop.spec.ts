import { expect, test, type Page } from '@playwright/test';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import type { CoreEvent, Reply, SessionRequest } from '../../src/ipc';
import type { backend } from '../../wailsjs/go/models';

declare global { interface Window { __r6Events: CoreEvent[] } }

async function rpc<T>(page: Page, method: string, argument?: unknown): Promise<T> {
  return page.evaluate(async ({ method, argument }) => {
    const api = (window as unknown as { go: { backend: { API: Record<string, (value?: unknown) => Promise<unknown>> } } }).go.backend.API;
    return argument === undefined ? await api[method]() : await api[method](argument);
  }, { method, argument }) as Promise<T>;
}

async function collectEvents(page: Page) {
  await page.evaluate(() => {
    window.__r6Events = [];
    const runtime = (window as unknown as { runtime: { EventsOnMultiple(name: string, callback: (event: CoreEvent) => void, limit: number): () => void } }).runtime;
    for (const name of ['session.status', 'changes.updated', 'session.warning', 'session.error']) {
      runtime.EventsOnMultiple(name, event => window.__r6Events.push(event), -1);
    }
  });
}

async function request(page: Page): Promise<SessionRequest> {
  await expect.poll(() => page.evaluate(() => window.__r6Events.some(e => e.status.state === 'Monitoring'))).toBe(true);
  return page.evaluate(() => {
    const event = [...window.__r6Events].reverse().find(e => e.status.state === 'Monitoring');
    if (!event) throw new Error('Missing real backend Monitoring notification');
    return { clientId: event.clientId, sessionId: event.status.sessionId };
  });
}

function checkConsole(page: Page): string[] {
  const problems: string[] = [];
  page.on('pageerror', error => problems.push(error.message));
  page.on('console', message => { if (['error', 'warning'].includes(message.type())) problems.push(message.text()); });
  page.on('response', response => { if (response.status() >= 400) problems.push(`${response.status()} ${response.url()}`); });
  return problems;
}

async function layout(page: Page) {
  expect(await page.locator('vite-error-overlay').count()).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await expect(page.getByRole('heading', { name: 'Keep an eye on your folder.' })).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'Folder path' })).toBeVisible();
}

test('real Wails IPC: visible Start/Stop, core changes/diff, pause/reset and responsive shell', async ({ page }, info) => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'folderwatch-r6-e2e-'));
  const root = path.join(temp, "项目 with spaces 'quote'");
  await mkdir(root);
  const key = "notes 世界 'quoted'.txt";
  await writeFile(path.join(root, key), 'PRIVATE_BEFORE_SENTINEL\n');
  const errors = checkConsole(page);
  try {
    await page.goto('/');
    await expect(page).toHaveTitle('FolderWatch');
    const app = await rpc<backend.AppInfo>(page, 'GetAppInfo');
    expect(app.name).toBe('FolderWatch');
    expect(app.protocol).toBe(1);
    expect(app.heartbeatMillis).toBe(2000);
    expect(app.leaseMillis).toBe(15000);
    await expect(page.getByRole('status')).toHaveText('Idle');
    await layout(page);
    await page.screenshot({ path: info.outputPath('desktop-idle.png'), fullPage: true });
    await collectEvents(page);
    await page.getByRole('textbox', { name: 'Folder path' }).fill(root);
    await page.getByRole('button', { name: 'Start monitoring' }).click();
    await expect(page.getByRole('status')).toHaveText('Monitoring');
    const req = await request(page);
    await writeFile(path.join(root, key), 'PRIVATE_AFTER_SENTINEL\n');
    await expect.poll(async () => (await rpc<backend.ChangesReply>(page, 'GetChanges', { ...req, offset: 0, limit: 10 })).total).toBe(1);
    const changes = await rpc<backend.ChangesReply>(page, 'GetChanges', { ...req, offset: 0, limit: 10 });
    expect(changes.error).toBeUndefined();
    expect(changes.changes[0].path).toBe(key);
    expect(changes.changes[0].kind).toBe('modified');
    const diff = await rpc<backend.DiffReply>(page, 'GetDiff', { ...req, path: key, generation: changes.generation, version: changes.changes[0].version });
    expect(diff.error).toBeUndefined();
    expect(diff.diff?.status).toBe('text');
    expect(JSON.stringify(diff.diff?.hunks)).toContain('PRIVATE_BEFORE_SENTINEL');
    expect(JSON.stringify(diff.diff?.hunks)).toContain('PRIVATE_AFTER_SENTINEL');
    await expect(page.locator('.file-row')).toHaveCount(1);
    await page.locator('.file-row').click();
    await expect(page.locator('.selected-file h2')).toHaveText(key);
    await expect.poll(() => page.locator('.monaco-host .view-lines').count()).toBeGreaterThan(0);
    await expect(page.locator('.monaco-host')).toContainText('PRIVATE_BEFORE_SENTINEL');
    await expect(page.locator('.monaco-host')).toContainText('PRIVATE_AFTER_SENTINEL');
    await page.screenshot({ path: info.outputPath('diff-visible.png'), fullPage: true });
    // Re-selecting recreates the loading state and must keep a live editor host.
    await page.locator('.file-row').click();
    await expect(page.locator('.monaco-host .view-lines')).not.toHaveCount(0);
    await expect(page.locator('.monaco-host')).toContainText('PRIVATE_AFTER_SENTINEL');
    expect(await page.evaluate(() => JSON.stringify(window.__r6Events))).not.toContain('PRIVATE_');
    const outside = await rpc<backend.DiffReply>(page, 'GetDiff', { ...req, path: '../outside', generation: changes.generation, version: changes.changes[0].version });
    expect(outside.error?.code).toBe('INVALID_PATH');
    expect((await rpc<Reply>(page, 'PauseSession', req)).status.state).toBe('Paused');
    await expect(page.getByRole('status')).toHaveText('Paused');
    await writeFile(path.join(root, key), 'PRIVATE_RESUMED_SENTINEL\n');
    expect((await rpc<Reply>(page, 'ResumeSession', req)).status.state).toBe('Monitoring');
    await expect(page.locator('.monaco-host')).toContainText('PRIVATE_RESUMED_SENTINEL');
    const reset = await rpc<Reply>(page, 'ResetBaseline', req);
    expect(reset.error).toBeUndefined();
    expect(BigInt(reset.status.generation)).toBeGreaterThan(BigInt(changes.generation));
    expect((await rpc<backend.ChangesReply>(page, 'GetChanges', { ...req, limit: 10 })).total).toBe(0);
    await page.screenshot({ path: info.outputPath('desktop-monitoring.png'), fullPage: true });
    await page.getByRole('button', { name: 'Stop session' }).click();
    await expect(page.getByRole('status')).toHaveText('Idle');
    for (let cycle = 0; cycle < 2; cycle++) {
      await page.getByRole('button', { name: 'Start monitoring' }).click();
      await expect(page.getByRole('status')).toHaveText('Monitoring');
      await page.getByRole('button', { name: 'Stop session' }).click();
      await expect(page.getByRole('status')).toHaveText('Idle');
    }
    await page.setViewportSize({ width: 680, height: 480 });
    await layout(page);
    await page.screenshot({ path: info.outputPath('minimum-window.png'), fullPage: true });
    await page.setViewportSize({ width: 380, height: 800 });
    await layout(page);
    await page.screenshot({ path: info.outputPath('narrow-css-stress.png'), fullPage: true });
    expect(errors).toEqual([]);
  } finally { await page.close(); await rm(temp, { recursive: true, force: true }); }
});

test('real Wails IPC: reload revokes old requests, input failure recovers, no fake running state', async ({ page }, info) => {
  const root = await mkdtemp(path.join(os.tmpdir(), 'folderwatch-r6-reload-'));
  const errors = checkConsole(page);
  try {
    await page.goto('/');
    await expect(page.getByRole('status')).toHaveText('Idle');
    await collectEvents(page);
    await page.getByRole('textbox', { name: 'Folder path' }).fill(root);
    await page.getByRole('button', { name: 'Start monitoring' }).click();
    await expect(page.getByRole('status')).toHaveText('Monitoring');
    const old = await request(page);
    await page.reload();
    await expect(page.getByRole('status')).toHaveText('Idle');
    expect((await rpc<Reply>(page, 'StopSession', old)).error?.code).toBe('STALE_CLIENT');
    await page.getByRole('textbox', { name: 'Folder path' }).fill('not-an-absolute-path');
    await page.getByRole('button', { name: 'Start monitoring' }).click();
    await expect(page.getByRole('alert')).toContainText('INVALID_ROOT');
    await expect(page.getByRole('status')).toHaveText('Idle');
    await page.screenshot({ path: info.outputPath('input-error.png'), fullPage: true });
    await page.getByRole('textbox', { name: 'Folder path' }).fill(root);
    await page.getByRole('button', { name: 'Start monitoring' }).click();
    await expect(page.getByRole('status')).toHaveText('Monitoring');
    await page.getByRole('button', { name: 'Stop session' }).click();
    await expect(page.getByRole('status')).toHaveText('Idle');
    expect(errors).toEqual([]);
  } finally { await page.close(); await rm(root, { recursive: true, force: true }); }
});
