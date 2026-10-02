import { defineConfig } from '@playwright/test';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const baseURL = process.env.FW_GUI_URL ?? 'http://127.0.0.1:34115';
const address = new URL(baseURL);
if (address.protocol !== 'http:' || !['localhost', '127.0.0.1'].includes(address.hostname)) {
  throw new Error('GUI smoke tests must target a loopback Wails development server.');
}

// Start `make gui-dev` separately. No mock backend and no production test RPCs.
// Local Chrome can be selected without downloading another browser.
export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 60000,
  expect: { timeout: 10000 },
  reporter: 'line',
  outputDir: process.env.FW_GUI_QA_DIR ?? path.join(os.tmpdir(), 'folderwatch-r6-gui-qa'),
  webServer: process.env.FW_GUI_START_SERVER === '1' ? {
    command: `../bin/wails dev -m -nosyncgomod -tags desktop -devserver ${address.host} -nogorebuild -noreload`,
    cwd: fileURLToPath(new URL('..', import.meta.url)),
    url: baseURL,
    timeout: 180000,
    reuseExistingServer: false,
    gracefulShutdown: { signal: 'SIGINT', timeout: 10000 },
  } : undefined,
  use: {
    baseURL,
    browserName: 'chromium',
    channel: process.env.FW_BROWSER_CHANNEL || undefined,
    headless: true,
    viewport: { width: 1040, height: 720 },
    colorScheme: 'light',
    screenshot: 'only-on-failure',
  },
});
