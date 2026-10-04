import { expect, test } from '@playwright/test';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

// Visible controls -> real Wails binding -> the same Go core, not a mocked list.
test('large folder: 9000 directories load and later edits reach the visible diff', async ({ page }, info) => {
  test.setTimeout(120000);
  const root = await mkdtemp(path.join(os.tmpdir(), 'folderwatch-large-gui-'));
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('console', message => { if (['error', 'warning'].includes(message.type())) errors.push(message.text()); });
  page.on('response', response => { if (response.status() >= 400) errors.push(`${response.status()} ${response.url()}`); });
  try {
    for (let i = 0; i < 9000; i++) await mkdir(path.join(root, `d-${String(i).padStart(5, '0')}`));
    const key = 'd-08999/中文 late.txt';
    await writeFile(path.join(root, key), 'large-tree baseline\n');
    await page.goto('/');
    await expect(page).toHaveTitle('FolderWatch');
    await expect(page.getByRole('status')).toHaveText('Idle');
    await page.getByRole('textbox', { name: 'Folder path' }).fill(root);
    await page.getByRole('button', { name: 'Start monitoring' }).click();
    await expect(page.getByRole('status')).toHaveText('Monitoring', { timeout: 60000 });
    await expect(page.locator('vite-error-overlay')).toHaveCount(0);
    await writeFile(path.join(root, key), 'large-tree independent edit\n');
    await expect(page.locator('.file-row')).toHaveCount(1);
    await page.locator('.file-row').click();
    await expect(page.locator('.selected-file h2')).toHaveText(key);
    await expect(page.locator('.monaco-host')).toContainText('large-tree baseline');
    await expect(page.locator('.monaco-host')).toContainText('large-tree independent edit');
    await mkdir(path.join(root, 'new', 'deep'), { recursive: true });
    await writeFile(path.join(root, 'new', 'deep', 'later.txt'), 'created after startup\n');
    await expect(page.locator('.file-row')).toHaveCount(2);
    await expect(page.getByRole('status')).toHaveText('Monitoring');
    // Monaco owns two role=alert accessibility live regions even without an
    // error. Check FolderWatch's error banners, not those editor internals.
    await expect(page.locator('.error[role="alert"]')).toHaveCount(0);
    await page.screenshot({ path: info.outputPath('large-folder-monitoring.png'), fullPage: true });
    await page.getByRole('button', { name: 'Stop session' }).click();
    await expect(page.getByRole('status')).toHaveText('Idle');
    expect(errors).toEqual([]);
  } finally {
    await page.close();
    await rm(root, { recursive: true, force: true });
  }
});
