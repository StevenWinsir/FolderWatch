// Stable transport types are defined by gui/backend/dto.go. bridge.ts consumes
// Wails-generated bindings so signature drift is caught by TypeScript checks.
export type State = 'Idle' | 'Scanning' | 'Monitoring' | 'Paused' | 'Stopping' | 'Error';
export interface Problem { code: string; message: string }
export interface SessionInfo {
  sessionId: string; root: string; state: string; sequence: string;
  generation: string; version: string; warning: string; problem?: Problem;
}
export interface AppInfo {
  name: string; version: string; commit: string; buildDate: string;
  protocol: number; heartbeatMillis: number; leaseMillis: number;
}
export interface Reply { status: SessionInfo; error?: Problem }
export interface ConnectionReply extends Reply { clientId: string; app: AppInfo }
export interface FolderReply { path: string; error?: Problem }
export interface SessionRequest { clientId: string; sessionId: string }
export interface StartOptions {
  clientId: string; root: string; debounce?: string; ignore?: string[];
  respectGitIgnore?: boolean; maxDiffBytes?: string; editor?: string;
}
export interface GUISettings { debounce: string; ignore: string[]; respectGitIgnore: boolean; maxDiffBytes: string; editor: string }
export interface SettingsRequest { clientId: string; root: string }
export interface SettingsReply { settings: GUISettings; error?: Problem }
export interface PathRequest extends SessionRequest { path: string }
export interface EditorRequest extends PathRequest { editor?: string }
export interface CopyPathRequest extends PathRequest { relative: boolean }
export interface CoreEvent {
  protocol: number; name: string; clientId: string;
  status: SessionInfo; reload: boolean; problem?: Problem;
}
export const eventNames = ['session.status', 'changes.updated', 'session.warning', 'session.error'] as const;
export interface FileInfo { sizeBytes: string; kind: string; classification: string; reason: string }
export interface ChangeSummary {
  path: string; oldPath: string; kind: string; version: string;
  before?: FileInfo; after?: FileInfo; firstSeen: string; lastSeen: string;
}
export interface ChangesRequest extends SessionRequest { offset: number; limit: number; generation: string; version: string }
export interface ChangesReply {
  sessionId: string; generation: string; version: string; changes: ChangeSummary[];
  total: number; nextOffset: number; error?: Problem;
}
export interface DiffRequest extends SessionRequest { path: string; generation: string; version: string }
export interface DiffLine { kind: string; oldLine: number; newLine: number; text: string; noNewline: boolean }
export interface DiffHunk { oldStart: number; oldLines: number; newStart: number; newLines: number; lines: DiffLine[] }
export interface DiffResult {
  sessionId: string; path: string; kind: string; status: string; reason: string;
  generation: string; version: string; hunks: DiffHunk[];
}
export interface DiffReply { diff?: DiffResult; error?: Problem }
export interface Bridge {
  available(): boolean;
  attach(): Promise<ConnectionReply>;
  detach(clientId: string): Promise<Reply>;
  heartbeat(clientId: string): Promise<Reply>;
  status(clientId: string): Promise<Reply>;
  start(options: StartOptions): Promise<Reply>;
  stop(request: SessionRequest): Promise<Reply>;
  pause?: (request: SessionRequest) => Promise<Reply>;
  resume?: (request: SessionRequest) => Promise<Reply>;
  reset?: (request: SessionRequest) => Promise<Reply>;
  settings?: (request: SettingsRequest) => Promise<SettingsReply>;
  openEditor?: (request: EditorRequest) => Promise<Reply>;
  reveal?: (request: PathRequest) => Promise<Reply>;
  copyPath?: (request: CopyPathRequest) => Promise<Reply>;
  selectFolder?: () => Promise<FolderReply>;
  changes?: (request: ChangesRequest) => Promise<ChangesReply>;
  diff?: (request: DiffRequest) => Promise<DiffReply>;
  on(name: string, callback: (event: CoreEvent) => void): () => void;
}
export const idle = (): SessionInfo => ({
  sessionId: '', root: '', state: 'Idle', sequence: '0', generation: '0', version: '0', warning: '',
});

export function isCounter(value: unknown): value is string {
  return typeof value === 'string' && /^(0|[1-9][0-9]*)$/.test(value) && value.length <= 20 && BigInt(value) <= 18446744073709551615n;
}
export function isSession(value: unknown): value is SessionInfo {
  if (!value || typeof value !== 'object') return false;
  const s = value as Record<string, unknown>;
  return ['Idle', 'Scanning', 'Monitoring', 'Paused', 'Stopping', 'Error'].includes(String(s.state))
    && ['sessionId', 'root', 'warning'].every(key => typeof s[key] === 'string')
    && [s.sequence, s.generation, s.version].every(isCounter);
}
