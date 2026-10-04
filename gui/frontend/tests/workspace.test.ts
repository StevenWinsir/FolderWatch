import { describe, expect, it, vi } from 'vitest';
import { get, writable } from 'svelte/store';
import { WorkspaceController } from '../src/workspace';
import type { SessionController } from '../src/controller';
import { idle, type Bridge, type ChangeSummary, type ChangesReply, type ChangesRequest, type CoreEvent, type DiffReply } from '../src/ipc';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(yes => { resolve = yes; });
  return { resolve, promise };
}
const change = (i: number, version = '1'): ChangeSummary => ({ path: `file-${String(i).padStart(4, '0')}.txt`, oldPath: '', kind: 'modified', version, firstSeen: '', lastSeen: '' });
function fixture(count = 1501) {
  let version = '1';
  const rows = Array.from({ length: count }, (_, i) => change(i));
  const state = writable({ status: { ...idle(), state: 'Monitoring', sessionId: 'session', generation: '1', version } });
  let listener: (event: CoreEvent) => void = () => {};
  const session = { state, clientId: () => 'client', subscribe: (callback: typeof listener) => { listener = callback; return vi.fn(); } } as unknown as SessionController;
  const changes = vi.fn(async (req: ChangesRequest): Promise<ChangesReply> => {
    if (req.offset && req.version !== version) return { sessionId: 'session', generation: '1', version, changes: [], total: rows.length, nextOffset: -1, error: { code: 'STALE_VERSION', message: 'changed' } };
    const matched = rows.filter(row => row.path.includes(req.filter ?? ''));
    return { sessionId: 'session', generation: '1', version, changes: matched.slice(req.offset, req.offset + req.limit), total: rows.length, matched: matched.length, nextOffset: req.offset + req.limit < matched.length ? req.offset + req.limit : -1 };
  });
  const diff = vi.fn(async (req): Promise<DiffReply> => ({ diff: { sessionId: req.sessionId, path: req.path, kind: 'modified', status: 'text', reason: '', generation: req.generation, version: req.version, hunks: [] } }));
  const bridge = { changes, diff } as unknown as Bridge;
  const workspace = new WorkspaceController(session, bridge);
  return { workspace, changes, diff, state, emit: () => listener({ name: 'changes.updated' } as CoreEvent), bump: () => { version = String(Number(version) + 1); state.update(s => ({ status: { ...s.status, version } })); } };
}

describe('bounded, versioned workspace pages', () => {
  it('makes the 501st and final changed file accessible without retaining earlier pages', async () => {
    const f = fixture();
    await f.workspace.refresh();
    expect(get(f.workspace.state).changes).toHaveLength(500);
    await f.workspace.nextPage();
    expect(get(f.workspace.state).changes[0].path).toBe('file-0500.txt');
    await f.workspace.nextPage();
    await f.workspace.nextPage();
    expect(get(f.workspace.state).changes).toHaveLength(1);
    expect(get(f.workspace.state).changes[0].path).toBe('file-1500.txt');
    await f.workspace.select(get(f.workspace.state).changes[0]);
    expect(get(f.workspace.state).diff?.path).toBe('file-1500.txt');
    await f.workspace.previousPage();
    expect(get(f.workspace.state).offset).toBe(1000);
    expect(get(f.workspace.state).selectedPath).toBe('');
    f.workspace.dispose();
  });

  it('searches the complete server index, not just the first 500 files', async () => {
    const f = fixture();
    await f.workspace.refresh();
    await f.workspace.setFilter('1500');
    expect(get(f.workspace.state).matched).toBe(1);
    expect(get(f.workspace.state).total).toBe(1501);
    expect(get(f.workspace.state).changes[0].path).toBe('file-1500.txt');
    await f.workspace.setFilter('not-present');
    expect(get(f.workspace.state).changes).toHaveLength(0);
    expect(get(f.workspace.state).matched).toBe(0);
    f.workspace.dispose();
  });

  it('restarts a stale continuation at page zero without merging generations', async () => {
    const f = fixture(); await f.workspace.refresh();
    f.bump();
    await f.workspace.nextPage();
    expect(get(f.workspace.state).offset).toBe(0);
    expect(get(f.workspace.state).version).toBe('2');
    expect(f.changes.mock.calls.map(([request]) => request.offset)).toEqual([0, 500, 0]);
    f.workspace.dispose();
  });

  it('coalesces invalidations and discards a late reply after search or disposal', async () => {
    const f = fixture();
    const pending = deferred<ChangesReply>();
    f.changes.mockImplementationOnce(() => pending.promise);
    const first = f.workspace.refresh();
    for (let i = 0; i < 100; i++) f.emit();
    const searched = f.workspace.setFilter('1500');
    expect(f.changes).toHaveBeenCalledTimes(1);
    pending.resolve({ sessionId: 'session', generation: '1', version: '1', changes: [change(0)], total: 1501, nextOffset: 500 });
    await first; await searched;
    expect(f.changes).toHaveBeenCalledTimes(2);
    expect(get(f.workspace.state).changes[0].path).toBe('file-1500.txt');
    const late = deferred<ChangesReply>(); f.changes.mockImplementationOnce(() => late.promise);
    const loading = f.workspace.refresh(); f.workspace.dispose();
    late.resolve({ sessionId: 'session', generation: '1', version: '1', changes: [change(0)], total: 1, nextOffset: -1 });
    await loading;
    expect(get(f.workspace.state).changes).toHaveLength(0);
  });

  it('runs only one diff RPC while a newer selection waits, and never displays the old result', async () => {
    const f = fixture(); await f.workspace.refresh();
    const pending = deferred<DiffReply>(); f.diff.mockImplementationOnce(() => pending.promise);
    const first = f.workspace.select(change(0));
    const second = f.workspace.select(change(1));
    expect(f.diff).toHaveBeenCalledTimes(1);
    pending.resolve({ diff: { sessionId: 'session', path: change(0).path, generation: '1', version: '1', kind: 'modified', status: 'text', reason: '', hunks: [] } });
    await first; await second;
    expect(f.diff).toHaveBeenCalledTimes(2);
    expect(get(f.workspace.state).diff?.path).toBe(change(1).path);
    f.workspace.dispose();
  });
});
