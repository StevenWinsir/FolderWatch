import { afterEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { SessionController } from '../src/controller';
import { idle, eventNames, isCounter, type Bridge, type CoreEvent, type ConnectionReply, type Reply, type SessionInfo } from '../src/ipc';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
function status(sequence = '1', state = 'Idle', sessionId = ''): SessionInfo {
  return { ...idle(), sequence, state, sessionId, root: sessionId ? '/fixture' : '' };
}
function fixture() {
  const callbacks = new Map<string, (event: CoreEvent) => void>();
  const removers = eventNames.map(() => vi.fn());
  const connection: ConnectionReply = { clientId: 'client', status: status(), app: { name: 'FolderWatch', version: 'test', commit: 'test', buildDate: 'test', protocol: 1, heartbeatMillis: 2000, leaseMillis: 15000 } };
  const bridge: Bridge = {
    available: vi.fn(() => true),
    attach: vi.fn(async () => connection),
    detach: vi.fn(async () => ({ status: idle() })),
    heartbeat: vi.fn(async () => ({ status: status() })),
    status: vi.fn(async () => ({ status: status() })),
    start: vi.fn(async () => ({ status: status('3', 'Monitoring', 'session') })),
    stop: vi.fn(async () => ({ status: status('5', 'Idle', 'session') })),
    on: vi.fn((name, callback) => { callbacks.set(name, callback); return removers[eventNames.indexOf(name as typeof eventNames[number])]; }),
  };
  const controller = new SessionController(bridge);
  const emit = (value: SessionInfo, client = 'client', name = 'session.status') => callbacks.get(name)?.({ protocol: 1, clientId: client, name, status: value, reload: true });
  return { bridge, controller, connection, emit, removers };
}

afterEach(() => { vi.useRealTimers(); });

describe('frontend lease and session adapter', () => {
  it('subscribes before attaching, hydrates and cleans up exactly once', async () => {
    const f = fixture();
    await f.controller.connect();
    expect(f.bridge.on).toHaveBeenCalledTimes(4);
    expect(vi.mocked(f.bridge.on).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(f.bridge.attach).mock.invocationCallOrder[0]);
    expect(get(f.controller.state).connected).toBe(true);
    f.controller.dispose(); f.controller.dispose();
    expect(f.bridge.detach).toHaveBeenCalledExactlyOnceWith('client');
    for (const remove of f.removers) expect(remove).toHaveBeenCalledTimes(1);
  });

  it('cleans up listeners when registration fails partway through', async () => {
    const f = fixture();
    const remove = vi.fn();
    vi.mocked(f.bridge.on).mockImplementationOnce(() => remove).mockImplementationOnce(() => { throw new Error('listener failed'); });
    await f.controller.connect();
    expect(remove).toHaveBeenCalledTimes(1);
    expect(get(f.controller.state).error).toContain('listener failed');
    expect(f.bridge.attach).not.toHaveBeenCalled();
    f.controller.dispose();
  });

  it('never substitutes a fake backend in a normal browser', async () => {
    const f = fixture(); vi.mocked(f.bridge.available).mockReturnValue(false);
    await f.controller.connect();
    expect(f.bridge.attach).not.toHaveBeenCalled();
    expect(get(f.controller.state).error).toContain('Desktop bridge unavailable');
    await f.controller.start('/fixture');
    expect(f.bridge.start).not.toHaveBeenCalled();
    f.controller.dispose();
  });

  it('disposes a connection whose attach RPC completes after unmount', async () => {
    const f = fixture(); const pending = deferred<ConnectionReply>();
    vi.mocked(f.bridge.attach).mockReturnValue(pending.promise);
    const connected = f.controller.connect();
    f.controller.dispose(); pending.resolve(f.connection); await connected;
    expect(f.bridge.detach).toHaveBeenCalledExactlyOnceWith('client');
    expect(get(f.controller.state).connected).toBe(false);
  });

  it('ignores stale clients, out-of-order status and imprecise numeric counters', async () => {
    const f = fixture(); await f.controller.connect();
    f.emit(status('9007199254740993', 'Monitoring', 'new-session'));
    f.emit(status('9007199254740992', 'Idle'));
    f.emit(status('9007199254740994', 'Error'), 'old-client');
    expect(get(f.controller.state).status.state).toBe('Monitoring');
    expect(get(f.controller.state).status.sequence).toBe('9007199254740993');
    f.emit({ ...status(), sequence: 'not-a-counter' });
    expect(get(f.controller.state).status.state).toBe('Monitoring');
    expect(isCounter(9007199254740993)).toBe(false);
    expect(isCounter('18446744073709551616')).toBe(false);
    f.controller.dispose();
  });

  it('does not roll a newer baseline back on an equal-sequence heartbeat', async () => {
    vi.useFakeTimers(); const f = fixture(); await f.controller.connect();
    f.emit({ ...status('5', 'Monitoring', 'session'), generation: '2', version: '8' });
    vi.mocked(f.bridge.heartbeat).mockResolvedValue({ status: { ...status('5', 'Monitoring', 'session'), generation: '1', version: '9' } });
    await vi.advanceTimersByTimeAsync(2000);
    expect(get(f.controller.state).status.generation).toBe('2');
    f.controller.dispose();
  });

  it('allows Stop while Start is scanning and rejects the late Start reply', async () => {
    const f = fixture(); await f.controller.connect();
    const pending = deferred<Reply>(); vi.mocked(f.bridge.start).mockReturnValue(pending.promise);
    const start = f.controller.start('/fixture');
    f.emit(status('2', 'Scanning', 'session'));
    await f.controller.stop();
    expect(f.bridge.stop).toHaveBeenCalledWith({ clientId: 'client', sessionId: 'session' });
    pending.resolve({ status: status('3', 'Monitoring', 'session') }); await start;
    expect(get(f.controller.state).status.state).toBe('Idle');
    expect(get(f.controller.state).pendingStart).toBe(false);
    await f.controller.start('/fixture'); // no second start queued during first RPC
    f.controller.dispose();
  });

  it('bounds heartbeat concurrency and disconnects on lease expiry', async () => {
    vi.useFakeTimers(); const f = fixture(); await f.controller.connect();
    const pending = deferred<Reply>(); vi.mocked(f.bridge.heartbeat).mockReturnValue(pending.promise);
    await vi.advanceTimersByTimeAsync(8000);
    expect(f.bridge.heartbeat).toHaveBeenCalledTimes(1);
    pending.resolve({ status: idle(), error: { code: 'STALE_CLIENT', message: 'Lease expired' } });
    await vi.advanceTimersByTimeAsync(1);
    expect(get(f.controller.state).connected).toBe(false);
    expect(get(f.controller.state).error).toContain('Lease expired');
    await vi.advanceTimersByTimeAsync(20000);
    expect(f.bridge.heartbeat).toHaveBeenCalledTimes(1);
    expect(f.bridge.detach).toHaveBeenCalledExactlyOnceWith('client');
    f.controller.dispose();
  });

  it('reports startup errors without treating them as a running session', async () => {
    const f = fixture(); await f.controller.connect();
    vi.mocked(f.bridge.start).mockResolvedValue({ status: idle(), error: { code: 'INVALID_ROOT', message: 'Choose a directory' } });
    await f.controller.start('relative');
    expect(get(f.controller.state).error).toContain('INVALID_ROOT');
    expect(get(f.controller.state).status.state).toBe('Idle');
    expect(get(f.controller.state).pendingStart).toBe(false);
    f.controller.dispose();
  });
});
