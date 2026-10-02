# FolderWatch GUI IPC v1

Authoritative definitions: `gui/backend/dto.go`, generated `gui/frontend/wailsjs/`, and the Go/TypeScript shared fixture in `gui/frontend/tests/fixtures/ipc-v1.json`.

## Transport and lifetime

The sole Wails binding is `backend.API`. Go errors do not escape as inconsistent exception strings: RPCs return `error?: {code, message}`; success omits that field. An unsuccessful call's status is not authoritative. Consumers must inspect `error` before applying a result. File paths and error messages are rendered as text, never HTML.

Call `AttachFrontend()` after registering the four event listeners. It returns `{clientId, app, status, error?}` and joins any previous session. Then call `GetSessionStatus(clientId)` to close the subscribe/attach race. `Heartbeat(clientId)` renews the frontend lease every 2 seconds; 15 seconds without renewal expires it. Heartbeats transfer only status metadata and do not poll the filesystem. Unmount/pagehide calls `DetachFrontend(clientId)` and removes every listener/timer; backend lease expiry is the fallback when that call cannot arrive. Delayed detachment from an old document is a no-op.

`OnShutdown` cancels and joins the facade and its event pump. Start/Stop, reload, startup cancellation and application shutdown share this ownership model. The frontend does not restore a session automatically after reload, disconnection or sleep-induced lease expiry. A 15-second lease is a cancellation trigger, not a hard OS I/O completion deadline.

## RPCs

| Method | Request | Success |
|---|---|---|
| `GetAppInfo` | none | Name, build version/commit/date, protocol and lease timings |
| `AttachFrontend` | none | New frontend capability, metadata and idle status |
| `DetachFrontend` | client ID | Idempotent detach/cleanup |
| `Heartbeat` | client ID | Renewed lease and current metadata |
| `GetSessionStatus` | client ID | Current session metadata |
| `StartSession` | client ID, explicit root, optional shared config overrides | Monitoring status after baseline capture; Scanning is notified while pending |
| `StopSession` | client ID + session ID | Idle after cleanup; also works during Scanning and is idempotent for the last stopped session |
| `PauseSession` / `ResumeSession` / `ResetBaseline` | client ID + session ID | Core-serialized result and current status |
| `GetChanges` | session request, offset, limit, optional generation/version | Summary page, total, nextOffset, core generation/version |
| `GetDiff` | session request, canonical relative path, generation, selected path version | Core-computed bounded hunks or metadata-only classification |

Settings/edit/reveal APIs are not placeholders in v1: they are deferred to P19–P20.

Start uses the same defaults < user config < project config < explicit overlay layering as the Terminal. Optional overrides are `debounce`, `ignore`, `respectGitIgnore`, and `maxDiffBytes`; omission inherits rather than inventing GUI defaults. A nil ignore list inherits; an explicit empty list clears that overlay. Initial root must be absolute or begin with `~/`, at most 32768 bytes, without NUL. There are at most 1024 explicit ignore patterns of 4096 bytes each.

Change pages default to 200 and have a hard limit of 500. `nextOffset = -1` ends the list. Every continuation page requires the first page's generation/version; `STALE_VERSION` means restart at offset zero. Changes contain relative paths, kinds, decimal byte sizes, classifications, timestamps and per-path versions. They contain no file text, private cache path or snapshot reference.

Diff requests require the **selected path's version**, not the list's global version. Responses carry the session ID, generation and path version again. The frontend must discard a response after selection/session/version changes. The facade also rechecks session/generation/path version before returning. Core byte/line/work budgets still apply. Only one diff request is admitted at a time; excess requests return `BUSY`. The facade adds a 15-second context and a conservative 16 MiB JSON wire bound, potentially falling back earlier than the configured core byte limit for huge/escaped lines. Fallback carries `status: "too-large"`, a reason and empty hunks.

## Events and ordering

`session.status`, `changes.updated`, `session.warning`, `session.error` share this envelope:

```json
{
  "protocol": 1,
  "name": "changes.updated",
  "clientId": "opaque-client-capability",
  "status": {
    "sessionId": "opaque-session-capability",
    "root": "/selected/root",
    "state": "Monitoring",
    "sequence": "42",
    "generation": "2",
    "version": "17",
    "warning": ""
  },
  "reload": true
}
```

States are Idle, Scanning, Monitoring, Paused, Stopping and Error. Optional `problem` carries a bounded code/message. Event queue capacity is 32; overflow discards intermediate invalidations, not the authoritative ChangeStore. Every event carries a complete metadata status and asks consumers to reload relevant data. Neither path deltas nor diff/file contents are pushed. Recoverable warnings are retained in status so losing a notification does not hide them; the next session clears them.

Reject events with another client ID or unsupported protocol. Compare sequence as a decimal integer (`BigInt` internally), not a JavaScript Number. Ignore duplicate/older events, and never roll generation/version back on an equal-sequence status response. A newer session can have generation/version reset to zero; its globally increasing sequence disambiguates it. Sequence gaps do not imply lost filesystem semantics: pull the current page/state. Do not interpret an empty invalidation as an empty list.

All uint64 counters and int64 byte counts are strings. Array counts, page offsets and line numbers are bounded JavaScript-safe integers. No serialized DTO embeds a Go core type.

## Error codes and path confinement

Common codes: `CLOSED`, `STALE_CLIENT`, `STALE_SESSION`, `CANCELLED`, `BUSY`, `INVALID_ROOT`, `INVALID_OPTIONS`, `INVALID_PAGE`, `INVALID_PATH`, `INVALID_VERSION`, `STALE_VERSION`, `NOT_CHANGED`, `TIMEOUT`, `CORE_ERROR`. Application failures are surfaced both as structured replies and, where applicable, status/error notifications.

Diff accepts only canonical slash-separated root-relative keys. Absolute paths, `..`, redundant separators/dot components, backslashes and NUL are rejected, not silently normalized. Unicode, spaces, quotes and platform-valid punctuation remain data. Ancestor symlinks are rejected; final symlinks receive metadata-only handling in the core. A deleted path may be diffed against retained baseline content when its ancestors remain safe. Replaced non-directory/symlink ancestors fail closed at this boundary. No shell command is formed from a root or file path.

These checks preserve the existing core policy; they do not claim atomic filesystem isolation against hostile ancestor replacement. The trusted native application can explicitly select another root through StartSession; capabilities distinguish document/session lifetime, not separate operating-system users.
