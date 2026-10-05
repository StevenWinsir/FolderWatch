import { describe, expect, it } from 'vitest';
import { shouldMountFrontend } from '../src/frontend-mode';

describe('browser IPC harness isolates only its development native window', () => {
  it('always mounts production, regardless of an accidentally inherited test flag', () => {
    for (const protocol of ['wails:', 'http:', 'https:', 'file:']) {
      expect(shouldMountFrontend(false, '1', protocol)).toBe(true);
    }
  });
  it('preserves normal native development without explicit browser-only mode', () => {
    expect(shouldMountFrontend(true, undefined, 'wails:')).toBe(true);
    expect(shouldMountFrontend(true, '0', 'wails:')).toBe(true);
  });
  it('mounts the real browser frontend but not a competing native client in E2E mode', () => {
    expect(shouldMountFrontend(true, '1', 'http:')).toBe(true);
    expect(shouldMountFrontend(true, '1', 'https:')).toBe(true);
    expect(shouldMountFrontend(true, '1', 'wails:')).toBe(false);
    expect(shouldMountFrontend(true, '1', 'file:')).toBe(false);
  });
});
