# ADR-006: Config and CLI contract

Status: Accepted (R1, 2026-10-01)

## Context
CLI, future TUI and GUI need shared defaults and deterministic overrides.

## Decision
Priority: built-in defaults < user TOML < root `.folderwatch.toml` < explicitly supplied CLI flags. Each higher `ignore` array replaces the lower array (including `[]` to clear); repeated CLI `--ignore` flags form the highest array. Ignore files remain separate/additive. Boolean false is an explicit override via `--flag=false`. Use `os.UserConfigDir()/FolderWatch/config.toml` (macOS: `~/Library/Application Support/FolderWatch/config.toml`). A supplied path defaults to cwd when omitted. CLI flags may occur before or after the path; `--` ends flag parsing.

Strict TOML rejects unknown keys and bad types. Validate each supplied layer so malformed lower-priority configuration is reported, not silently hidden by flags. Relative config paths resolve beside the declaring TOML file; CLI paths resolve against cwd. Expand `~` or `~/...`, not `~otheruser` or environment variables. Resolve root symlinks. An explicitly supplied ignore file must exist and be a regular readable file. A log-file destination is normalized/validated but not created in R1. Editor text is stored only, never executed or shell-expanded.

Durations use positive Go durations (default 150ms); sizes use positive whole bytes or decimal/binary units B/KB/MB/GB/TB and KiB/MiB/GiB/TiB, case-insensitively. Fractions are accepted only if the final byte count is an exact positive int64 (e.g. 1.5MiB). Max-diff default is 5MiB. Reject overflow, zero, negative and unsupported units.

`--help` and `--version` exit without loading config or scanning. Exit 0 means success (possibly with warnings), 2 invalid arguments/config, 1 runtime or output I/O failure, 130 cancellation. R1 `--scan` returns a deterministic metadata inventory; `--json` is scan JSON. The default is also one-shot until R4. `--no-mouse`, `--debounce`, `--max-diff-bytes`, `--editor`, `--log-file` and `--debug` are validated forward contracts, not implemented watcher/TUI/editor/logging features.

## Alternatives
Appending ignore arrays across all config layers prevents reliable clearing. Cwd-relative paths in user config change meaning between invocations. Silent unknown keys hide typos.

## Consequences
No config initialization or persistence is performed automatically. Config files and ignore files have a 1 MiB read limit. A single bad config layer fails clearly. Help documents reserved options and scan-only status.

## Migration
Keep this schema shared with future adapters. New resource limits should be introduced only when their owning stage implements them, with defaults/tests/docs in the same change.
