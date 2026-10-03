import * as API from '../wailsjs/go/backend/API';
import { EventsOn } from '../wailsjs/runtime/runtime';
import type { Bridge, CoreEvent } from './ipc';

// No browser mock is silently substituted for the desktop backend.
export const bridge: Bridge = {
  available: () => typeof window !== 'undefined' && Boolean((window as unknown as { go?: { backend?: { API?: unknown } } }).go?.backend?.API),
  attach: API.AttachFrontend,
  detach: API.DetachFrontend,
  heartbeat: API.Heartbeat,
  status: API.GetSessionStatus,
  start: API.StartSession,
  stop: API.StopSession,
  pause: API.PauseSession,
  resume: API.ResumeSession,
  reset: API.ResetBaseline,
  settings: API.GetSettings,
  openEditor: API.OpenInEditor,
  reveal: API.RevealInFinder,
  copyPath: API.CopyPath,
  selectFolder: API.SelectFolder,
  changes: API.GetChanges,
  diff: API.GetDiff,
  on: (name, callback) => EventsOn(name, (event: CoreEvent) => callback(event)),
};
