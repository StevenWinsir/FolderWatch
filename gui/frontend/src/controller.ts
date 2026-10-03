import { writable, type Readable } from 'svelte/store';
import { eventNames, idle, isSession, type AppInfo, type Bridge, type CoreEvent, type SessionInfo, type Reply, type StartOptions, type GUISettings } from './ipc';

export interface ViewState {
  connected: boolean; connecting: boolean; pendingStart: boolean;
  status: SessionInfo; app?: AppInfo; error: string;
}

// This store is an adapter, not a ChangeStore. It owns only connection/UI state;
// authoritative session state and all file semantics remain in the Go core.
export class SessionController {
  private data: ViewState = { connected: false, connecting: false, pendingStart: false, status: idle(), error: '' };
  private store = writable(this.data);
  readonly state: Readable<ViewState> = { subscribe: this.store.subscribe };
  private client = '';
  private epoch = 0;
  private disposed = false;
  private listeners: (() => void)[] = [];
  private timer?: ReturnType<typeof setInterval>;
  private retry?: ReturnType<typeof setTimeout>;
  private beating = false;
  private stopping = false;
  private subscribers = new Set<(event: CoreEvent) => void>();

  constructor(private readonly bridge: Bridge) {}
  private patch(update: Partial<ViewState>) { this.data = { ...this.data, ...update }; this.store.set(this.data); }
  private current(epoch: number, client = this.client) { return !this.disposed && this.epoch === epoch && this.client === client; }
  private apply(status: SessionInfo, equal = false) {
    if (!isSession(status)) throw new Error('The backend returned an invalid session status.');
    const previous = this.data.status;
    const order = BigInt(status.sequence) - BigInt(previous.sequence);
    if (order < 0n || (!equal && order === 0n)) return;
    if (status.sessionId === previous.sessionId && (BigInt(status.generation) < BigInt(previous.generation)
      || (status.generation === previous.generation && BigInt(status.version) < BigInt(previous.version)))) return;
    this.patch({ status });
  }
  private check(reply: Reply) {
    if (reply.error) throw new Error(`${reply.error.code}: ${reply.error.message}`);
    return reply.status;
  }
  private message(error: unknown) { return error instanceof Error ? error.message : String(error); }
  private unlisten() {
    if (this.timer) clearInterval(this.timer);
    if (this.retry) clearTimeout(this.retry);
    this.timer = undefined;
    this.retry = undefined;
    this.listeners.splice(0).forEach(remove => remove());
  }
  private scheduleRetry() {
    if (!this.disposed && !this.retry) this.retry = setTimeout(() => { this.retry = undefined; void this.connect(); }, 180);
  }
  private receive(event: CoreEvent) {
    if (this.disposed || !this.client || event?.protocol !== 1 || event.clientId !== this.client) return;
    if (!eventNames.includes(event.name as typeof eventNames[number]) || !isSession(event.status)) return;
    this.apply(event.status);
    this.subscribers.forEach(subscriber => subscriber(event));
  }

  subscribe(listener: (event: CoreEvent) => void) {
    this.subscribers.add(listener);
    return () => this.subscribers.delete(listener);
  }

  clientId() { return this.client; }
  setError(error: string) { if (!this.disposed) this.patch({ error }); }

  async connect() {
    if (this.disposed || this.data.connecting || this.data.connected) return;
    if (!this.bridge.available()) {
      this.patch({ error: 'Desktop bridge unavailable. Launch FolderWatch with make gui-dev or open the built .app.' });
      this.scheduleRetry();
      return;
    }
    const epoch = ++this.epoch;
    this.unlisten();
    this.patch({ connecting: true, error: '', status: idle() });
    try {
      for (const name of eventNames) this.listeners.push(this.bridge.on(name, event => this.receive(event)));
      const reply = await this.bridge.attach();
      if (this.disposed || epoch !== this.epoch) {
        if (reply.clientId) void this.bridge.detach(reply.clientId).catch(() => {});
        return;
      }
      this.check(reply);
      if (reply.app.protocol !== 1 || !reply.clientId || reply.app.heartbeatMillis < 500 || reply.app.heartbeatMillis > 10000) {
        if (reply.clientId) void this.bridge.detach(reply.clientId).catch(() => {});
        throw new Error('Unsupported FolderWatch IPC protocol.');
      }
      this.client = reply.clientId;
      this.apply(reply.status, true);
      this.patch({ connected: true, app: reply.app, connecting: false });
      // Status read closes the subscribe/attach race. Heartbeat exchanges only
      // lease/status metadata; it never polls the filesystem or fetches content.
      this.timer = setInterval(() => void this.heartbeat(epoch), reply.app.heartbeatMillis);
      const status = await this.bridge.status(this.client);
      if (this.current(epoch)) this.apply(this.check(status), true);
    } catch (error) {
      if (this.current(epoch)) {
        this.disconnect(this.message(error));
        this.scheduleRetry();
      }
    } finally {
      if (this.current(epoch)) this.patch({ connecting: false });
    }
  }

  private disconnect(error: string) {
    const old = this.client;
    this.client = '';
    this.epoch++;
    this.unlisten();
    this.beating = false;
    this.stopping = false;
    this.patch({ connected: false, connecting: false, pendingStart: false, status: idle(), error });
    if (old) void this.bridge.detach(old).catch(() => {}); // backend lease is the fallback
  }
  private async heartbeat(epoch: number) {
    if (this.beating || !this.current(epoch) || !this.client) return;
    this.beating = true;
    const client = this.client;
    try {
      const reply = await this.bridge.heartbeat(client);
      if (this.current(epoch, client)) this.apply(this.check(reply), true);
    } catch (error) {
      if (this.current(epoch, client)) this.disconnect(`Connection lost. ${this.message(error)}`);
    } finally { if (this.current(epoch, client)) this.beating = false; }
  }

  async start(root: string, settings?: Partial<GUISettings>) {
    if (this.disposed || !this.data.connected || this.data.pendingStart || !['Idle', 'Error'].includes(this.data.status.state)) return;
    const epoch = this.epoch;
    const client = this.client;
    this.patch({ pendingStart: true, error: '' });
    try {
      const options: StartOptions = { clientId: client, root };
      if (settings?.debounce) options.debounce = settings.debounce;
      if (settings?.ignore) options.ignore = settings.ignore;
      if (settings?.respectGitIgnore !== undefined) options.respectGitIgnore = settings.respectGitIgnore;
      if (settings?.maxDiffBytes) options.maxDiffBytes = settings.maxDiffBytes;
      if (settings?.editor !== undefined) options.editor = settings.editor;
      const reply = await this.bridge.start(options);
      if (!this.current(epoch, client)) return;
      if (reply.error?.code === 'CANCELLED') return; // intentional Stop during scan
      this.apply(this.check(reply), true);
    } catch (error) { if (this.current(epoch, client)) this.patch({ error: this.message(error) }); }
    finally { if (this.current(epoch, client)) this.patch({ pendingStart: false }); }
  }

  async stop() {
    const sessionId = this.data.status.sessionId;
    if (this.disposed || !this.data.connected || this.stopping || !sessionId || !['Scanning', 'Monitoring', 'Paused'].includes(this.data.status.state)) return;
    const epoch = this.epoch;
    const client = this.client;
    this.stopping = true;
    try {
      const reply = await this.bridge.stop({ clientId: client, sessionId });
      if (this.current(epoch, client)) this.apply(this.check(reply), true);
    } catch (error) { if (this.current(epoch, client)) this.patch({ error: this.message(error) }); }
    finally { if (this.current(epoch, client)) this.stopping = false; }
  }

  private async control(action: 'pause' | 'resume' | 'reset') {
    const method = this.bridge[action];
    const sessionId = this.data.status.sessionId;
    if (!method || this.disposed || !this.data.connected || !sessionId) return;
    const epoch = this.epoch;
    const client = this.client;
    try {
      const reply = await method({ clientId: client, sessionId });
      if (this.current(epoch, client)) this.apply(this.check(reply), true);
    } catch (error) { if (this.current(epoch, client)) this.patch({ error: this.message(error) }); }
  }
  pause() { return this.control('pause'); }
  resume() { return this.control('resume'); }
  reset() { return this.control('reset'); }

  async loadSettings(root: string) {
    if (!this.bridge.settings || !this.client || !root.trim()) return undefined;
    try {
      const reply = await this.bridge.settings({ clientId: this.client, root: root.trim() });
      if (reply.error) throw new Error(`${reply.error.code}: ${reply.error.message}`);
      return reply.settings;
    } catch (error) { this.setError(this.message(error)); return undefined; }
  }

  async openEditor(path: string, editor = '') { return this.systemAction('openEditor', { path, editor }); }
  async reveal(path: string) { return this.systemAction('reveal', { path }); }
  async copyPath(path: string, relative: boolean) { return this.systemAction('copyPath', { path, relative }); }
  private async systemAction(action: 'openEditor' | 'reveal' | 'copyPath', value: { path: string; editor?: string; relative?: boolean }) {
    const method = this.bridge[action];
    const sessionId = this.data.status.sessionId;
    if (!method || !this.client || !sessionId) return;
    try {
      const reply = await method({ clientId: this.client, sessionId, ...value } as never);
      if (reply.error) throw new Error(`${reply.error.code}: ${reply.error.message}`);
    } catch (error) { this.setError(this.message(error)); }
  }

  async wake() {
    if (this.disposed) return;
    if (this.data.connected) await this.heartbeat(this.epoch);
    else await this.connect();
  }

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    this.epoch++;
    this.unlisten();
    this.subscribers.clear();
    const old = this.client;
    this.client = '';
    if (old) void this.bridge.detach(old).catch(() => {});
  }
}
