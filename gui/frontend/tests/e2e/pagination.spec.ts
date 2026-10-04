import { expect, test } from '@playwright/test';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

test('real Wails IPC: every page and search beyond 500 remain accessible with a bounded DOM', async ({ page }, info) => {
  test.setTimeout(120000);
  const root = await mkdtemp(path.join(os.tmpdir(), 'folderwatch-paging-e2e-'));
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('console', entry => { if (['error', 'warning'].includes(entry.type())) errors.push(entry.text()); });
  page.on('response', response => { if (response.status() >= 400) errors.push(`${response.status()} ${response.url()}`); });
  const filename = (i: number) => `file-${String(i).padStart(4, '0')}.txt`;
  try {
    for (let i = 0; i < 1001; i++) await writeFile(path.join(root, filename(i)), 'original baseline\n');
    await page.goto('/');
    await expect(page.getByRole('status')).toHaveText('Idle');
    await page.getByRole('textbox', { name: 'Folder path' }).fill(root);
    await page.getByRole('button', { name: 'Start monitoring' }).click();
    await expect(page.getByRole('status')).toHaveText('Monitoring');
    await page.getByRole('button', { name: 'Pause', exact: true }).click();
    await expect(page.getByRole('status')).toHaveText('Paused');
    for (let i = 0; i < 1001; i++) await writeFile(path.join(root, filename(i)), `changed content ${i}\n`);
    await page.getByRole('button', { name: 'Resume', exact: true }).click();
    await expect(page.getByRole('status')).toHaveText('Monitoring');
    await expect(page.locator('.count-badge')).toHaveText('1001');
    await expect(page.locator('.file-row')).toHaveCount(500);
    await page.getByRole('button', { name: 'Next change page' }).click();
    await expect(page.locator('.file-row').first()).toContainText('file-0500.txt');
    await expect(page.locator('.file-row')).toHaveCount(500);
    await page.getByRole('button', { name: 'Next change page' }).click();
    await expect(page.locator('.file-row')).toHaveCount(1);
    await expect(page.locator('.file-row')).toContainText('file-1000.txt');
    await page.locator('.file-row').click();
    await expect(page.locator('.monaco-host')).toContainText('original baseline');
    await expect(page.locator('.monaco-host')).toContainText('changed content 1000');
    await page.getByRole('button', { name: 'Previous change page' }).click();
    await expect(page.locator('.file-row').first()).toContainText('file-0500.txt');
    await page.getByRole('textbox', { name: 'Filter changed files' }).fill('file-1000');
    await expect(page.locator('.file-row')).toHaveCount(1);
    await expect(page.locator('.file-row')).toContainText('file-1000.txt');
    await page.locator('.file-row').click();
    await expect(page.locator('.monaco-host')).toContainText('changed content 1000');
    await page.screenshot({ path: info.outputPath('last-page-search-diff.png'), fullPage: true });
    await page.setViewportSize({ width: 680, height: 480 });
    await expect.poll(() => page.evaluate(() => ({ width: innerWidth, scroll: document.documentElement.scrollWidth,
      overflowing: Array.from(document.querySelectorAll('main *')).map(element => ({ name: element.className, right: element.getBoundingClientRect().right })).filter(item => item.right > innerWidth + 1).slice(0, 10),
    })), { message: 'Active layout must settle without horizontal overflow after resize' }).toMatchObject({ width: 680, scroll: 680 });
    await expect(page.locator('.error[role="alert"]')).toHaveCount(0);
    expect(errors).toEqual([]);
    await page.getByRole('button', { name: 'Stop session' }).click();
    await expect(page.getByRole('status')).toHaveText('Idle');
  } finally {
    await page.close();
    await rm(root, { recursive: true, force: true });
  }
});
