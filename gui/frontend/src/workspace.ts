import { get, writable, type Readable } from 'svelte/store';
import type { Bridge, ChangeSummary, CoreEvent, DiffResult, SessionInfo } from './ipc';
import type { SessionController } from './controller';

export interface WorkspaceState {
  changes: ChangeSummary[];
  total: number;
  matched: number;
  offset: number;
  nextOffset: number;
  filter: string;
  generation: string;
  version: string;
  selectedPath: string;
  selected?: ChangeSummary;
  diff?: DiffResult;
  loading: boolean;
  loadingDiff: boolean;
  error: string;
}

const empty: WorkspaceState = {
  changes: [], total: 0, matched: 0, offset: 0, nextOffset: -1, filter: '',
  generation: '0', version: '0', selectedPath: '',
  loading: false, loadingDiff: false, error: '',
};
const queryStates = ['Monitoring', 'Paused'];
const activeStates = ['Scanning', 'Monitoring', 'Paused', 'Stopping'];
const pageSize = 500;
function message(error: unknown) { return error instanceof Error ? error.message : String(error); }

// One bounded page, one list RPC, and one diff RPC. Invalidation bursts coalesce
// into a single follow-up read. Search is server-side over the complete index;
// no filesystem logic or baseline is duplicated in the frontend.
export class WorkspaceController {
  private data: WorkspaceState = { ...empty };
  private store = writable(this.data);
  readonly state: Readable<WorkspaceState> = { subscribe: this.store.subscribe };
  private unsubscribe?: () => void;
  private viewEpoch = 0;
  private diffEpoch = 0;
  private disposed = false;
  private pendingRefresh = false;
  private refreshing?: Promise<void>;
  private wantedOffset = 0;
  private query = '';
  private sessionID = '';
  private pendingDiff = false;
  private diffRequest?: Promise<void>;

  constructor(private readonly session: SessionController, private readonly bridge: Bridge) {
    this.unsubscribe = session.subscribe((event: CoreEvent) => {
      const status = get(session.state).status; // ignore stale event status snapshots
      if (!activeStates.includes(status.state)) { this.clear(); return; }
      if ((event.name === 'changes.updated' || event.name === 'session.status') && queryStates.includes(status.state)
        && (status.sessionId !== this.sessionID || status.generation !== this.data.generation || status.version !== this.data.version)) {
        void this.refresh();
      }
    });
  }

  private patch(update: Partial<WorkspaceState>) {
    this.data = { ...this.data, ...update };
    this.store.set(this.data);
  }
  private request(session: SessionInfo) {
    return { clientId: this.session.clientId(), sessionId: session.sessionId };
  }
  private current(epoch: number, sessionID: string, clientID: string) {
    const status = get(this.session.state).status;
    return !this.disposed && epoch === this.viewEpoch && status.sessionId === sessionID
      && this.session.clientId() === clientID && queryStates.includes(status.state);
  }

  refresh(): Promise<void> {
    if (this.disposed || !this.bridge.changes) return Promise.resolve();
    this.pendingRefresh = true;
    if (!this.refreshing) {
      this.refreshing = this.drainRefresh().finally(() => {
        this.refreshing = undefined;
        if (this.pendingRefresh && !this.disposed) return this.refresh();
      });
    }
    return this.refreshing;
  }

  private async drainRefresh() {
    while (this.pendingRefresh && !this.disposed) {
      this.pendingRefresh = false;
      const session = get(this.session.state).status;
      if (!session.sessionId || !queryStates.includes(session.state)) { this.clear(); return; }
      if (session.sessionId !== this.sessionID) {
        this.sessionID = session.sessionId;
        this.wantedOffset = 0;
        this.viewEpoch++;
        this.invalidateSelection();
      }
      const epoch = this.viewEpoch;
      const request = this.request(session);
      const filter = this.query;
      let offset = this.wantedOffset;
      this.patch({ loading: true, error: '' });
      try {
        // An edited list invalidates a continuation. Retry the first page once,
        // never stitch together entries from different versions or retry forever.
        for (let attempt = 0; attempt < 2; attempt++) {
          const reply = await this.bridge.changes!({
            ...request, offset, limit: pageSize, filter,
            generation: offset ? this.data.generation : '', version: offset ? this.data.version : '',
          });
          if (!this.current(epoch, session.sessionId, request.clientId)) break;
          if (reply.error && ['STALE_VERSION', 'INVALID_PAGE'].includes(reply.error.code) && attempt === 0) {
            offset = 0;
            this.wantedOffset = 0;
            continue;
          }
          if (reply.error) throw new Error(`${reply.error.code}: ${reply.error.message}`);
          if (reply.sessionId !== session.sessionId || reply.changes.length > pageSize
            || (reply.nextOffset !== -1 && reply.nextOffset <= offset)) {
            throw new Error('INVALID_PAGE: The backend returned an inconsistent page.');
          }
          if (BigInt(reply.generation) < BigInt(this.data.generation)
            || reply.generation === this.data.generation && BigInt(reply.version) < BigInt(this.data.version)) break;
          const head = get(this.session.state).status;
          if (BigInt(reply.generation) < BigInt(head.generation)
            || reply.generation === head.generation && BigInt(reply.version) < BigInt(head.version)) {
            this.pendingRefresh = true;
            this.wantedOffset = 0;
            break;
          }
          const selected = reply.changes.find(change => change.path === this.data.selectedPath);
          const changedSelection = selected && (selected.version !== this.data.selected?.version || reply.generation !== this.data.generation);
          this.patch({ changes: reply.changes, total: reply.total, matched: reply.matched ?? reply.total,
            offset, nextOffset: reply.nextOffset, filter, generation: reply.generation,
            version: reply.version, selected, loading: false });
          if (!selected) this.invalidateSelection();
          else if (changedSelection) { this.patch({ diff: undefined }); void this.loadDiff(); }
          break;
        }
      } catch (error) {
        if (this.current(epoch, session.sessionId, request.clientId)) this.patch({ error: message(error) });
      } finally {
        if (this.current(epoch, session.sessionId, request.clientId)) this.patch({ loading: false });
      }
    }
  }

  setFilter(value: string): Promise<void> {
    if (value === this.query) return Promise.resolve();
    this.query = value;
    this.wantedOffset = 0;
    this.viewEpoch++;
    this.invalidateSelection();
    return this.refresh();
  }
  nextPage(): Promise<void> {
    if (this.data.loading || this.data.nextOffset < 0) return Promise.resolve();
    this.wantedOffset = this.data.nextOffset;
    this.viewEpoch++;
    return this.refresh();
  }
  previousPage(): Promise<void> {
    if (this.data.loading || this.data.offset === 0) return Promise.resolve();
    this.wantedOffset = Math.max(0, this.data.offset - pageSize);
    this.viewEpoch++;
    return this.refresh();
  }

  async select(change: ChangeSummary) {
    if (this.disposed) return;
    this.patch({ selectedPath: change.path, selected: change, diff: undefined, error: '' });
    await this.loadDiff();
  }
  private loadDiff(): Promise<void> {
    this.diffEpoch++;
    this.pendingDiff = true;
    if (!this.diffRequest) this.diffRequest = this.drainDiff().finally(() => {
      this.diffRequest = undefined;
      if (this.pendingDiff && !this.disposed) return this.loadDiff();
    });
    return this.diffRequest;
  }
  private async drainDiff() {
    while (this.pendingDiff && !this.disposed) {
      this.pendingDiff = false;
      const selected = this.data.selected;
      const session = get(this.session.state).status;
      if (!selected || !session.sessionId || !this.bridge.diff) return;
      const epoch = this.diffEpoch;
      const generation = this.data.generation;
      const request = this.request(session);
      this.patch({ loadingDiff: true, error: '' });
      const current = () => !this.disposed && epoch === this.diffEpoch
        && this.data.selectedPath === selected.path && this.data.selected?.version === selected.version
        && this.data.generation === generation && get(this.session.state).status.sessionId === request.sessionId
        && this.session.clientId() === request.clientId;
      try {
        const reply = await this.bridge.diff({ ...request, path: selected.path, generation, version: selected.version });
        if (!current()) continue;
        if (reply.error) throw new Error(`${reply.error.code}: ${reply.error.message}`);
        if (!reply.diff || reply.diff.generation !== generation || reply.diff.version !== selected.version) {
          throw new Error('STALE_VERSION: The selected file changed. Select it again.');
        }
        this.patch({ diff: reply.diff, loadingDiff: false });
      } catch (error) {
        if (current()) this.patch({ loadingDiff: false, error: message(error) });
      }
    }
  }
  private invalidateSelection() {
    this.diffEpoch++;
    this.pendingDiff = false;
    this.patch({ selectedPath: '', selected: undefined, diff: undefined, loadingDiff: false });
  }
  clear() {
    this.viewEpoch++;
    this.diffEpoch++;
    this.pendingRefresh = false;
    this.pendingDiff = false;
    this.sessionID = '';
    this.wantedOffset = 0;
    this.query = '';
    this.patch({ ...empty, selected: undefined, diff: undefined });
  }
  dispose() {
    if (this.disposed) return;
    this.clear();
    this.disposed = true;
    this.unsubscribe?.();
  }
}
