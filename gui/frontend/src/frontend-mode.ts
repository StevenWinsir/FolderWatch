// Wails dev starts a native WebView as well as its browser IPC server. Browser
// E2E must not mount a second, delayed native client that revokes Playwright's
// single-frontend capability. Production and ordinary development always mount.
export function shouldMountFrontend(development: boolean, browserOnly: string | undefined, protocol: string): boolean {
  return !development || browserOnly !== '1' || protocol === 'http:' || protocol === 'https:';
}
