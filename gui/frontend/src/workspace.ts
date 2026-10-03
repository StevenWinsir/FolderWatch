import { get, writable, type Readable } from 'svelte/store';
import type { Bridge, ChangeSummary, CoreEvent, DiffResult, SessionInfo } from './ipc';
import type { SessionController } from './controller';

export interface WorkspaceState {
  changes: ChangeSummary[];
  total: number;
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
  changes: [], total: 0, generation: '0', version: '0', selectedPath: '',
  loading: false, loadingDiff: false, error: '',
};

const activeStates = ['Scanning', 'Monitoring', 'Paused', 'Stopping'];
const queryStates = ['Monitoring', 'Paused'];

function message(error: unknown) { return error instanceof Error ? error.message : String(error); }

// This controller only owns view state and request epochs. Change semantics,
// versions and all file reads remain authoritative in the Go Facade.
export class WorkspaceController {
  private data = empty;
  private store = writable(this.data);
  readonly state: Readable<WorkspaceState> = { subscribe: this.store.subscribe };
  private unsubscribe?: () => void;
  private refreshEpoch = 0;
  private diffEpoch = 0;
  private disposed = false;

  constructor(private readonly session: SessionController, private readonly bridge: Bridge) {
    this.unsubscribe = session.subscribe((event: CoreEvent) => {
      if (event.name === 'changes.updated' || event.name === 'session.status') void this.refresh();
      if (!activeStates.includes(event.status.state)) this.clear();
    });
  }

  private patch(update: Partial<WorkspaceState>) {
    this.data = { ...this.data, ...update };
    this.store.set(this.data);
  }

  private request(session: SessionInfo) {
    return { clientId: this.clientID(), sessionId: session.sessionId };
  }

  // SessionController intentionally keeps its client capability private. The
  // backend request only needs the session id and the bridge can expose the
  // capability through this narrow optional hook in desktop builds.
  private clientID() {
    return this.session.clientId();
  }

  async refresh() {
    if (this.disposed || !this.bridge.changes) return;
    const session = get(this.session.state).status;
    if (!session.sessionId || !queryStates.includes(session.state)) {
      this.clear();
      return;
    }
    const epoch = ++this.refreshEpoch;
    this.patch({ loading: true, error: '' });
    try {
      const reply = await this.bridge.changes({
        ...this.request(session), offset: 0, limit: 500,
        generation: '', version: '',
      });
      if (epoch !== this.refreshEpoch || this.disposed) return;
      if (reply.error) throw new Error(`${reply.error.code}: ${reply.error.message}`);
      const selectedPath = this.data.selectedPath;
      const selected = reply.changes.find(change => change.path === selectedPath);
      this.patch({ changes: reply.changes, total: reply.total, generation: reply.generation, version: reply.version, selected, loading: false });
      if (!selected) {
        this.patch({ selectedPath: '', diff: undefined, loadingDiff: false });
      } else if (this.data.diff && (this.data.diff.version !== selected.version || this.data.diff.generation !== reply.generation)) {
        await this.loadDiff(selected);
      }
    } catch (error) {
      if (epoch === this.refreshEpoch && !this.disposed) this.patch({ loading: false, error: message(error) });
    }
  }

  async select(change: ChangeSummary) {
    this.patch({ selectedPath: change.path, selected: change, diff: undefined, error: '' });
    await this.loadDiff(change);
  }

  private async loadDiff(change: ChangeSummary) {
    if (!this.bridge.diff) return;
    const session = get(this.session.state).status;
    if (!session.sessionId) return;
    const epoch = ++this.diffEpoch;
    this.patch({ loadingDiff: true, error: '' });
    try {
      const reply = await this.bridge.diff({
        ...this.request(session), path: change.path,
        generation: this.data.generation, version: change.version,
      });
      if (epoch !== this.diffEpoch || this.disposed || this.data.selectedPath !== change.path) return;
      if (reply.error) throw new Error(`${reply.error.code}: ${reply.error.message}`);
      if (!reply.diff || reply.diff.generation !== this.data.generation || reply.diff.version !== change.version) {
        throw new Error('STALE_VERSION: The selected file changed. Select it again.');
      }
      this.patch({ diff: reply.diff, loadingDiff: false });
    } catch (error) {
      if (epoch === this.diffEpoch && !this.disposed && this.data.selectedPath === change.path) {
        this.patch({ loadingDiff: false, error: message(error) });
      }
    }
  }

  clear() {
    this.refreshEpoch++;
    this.diffEpoch++;
    this.patch({ changes: [], total: 0, generation: '0', version: '0', selectedPath: '', selected: undefined, diff: undefined, loading: false, loadingDiff: false });
  }

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    this.refreshEpoch++;
    this.diffEpoch++;
    this.unsubscribe?.();
  }
}
