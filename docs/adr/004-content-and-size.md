# ADR-004: Text, binary and large files

Status: Accepted; classification/diff implemented in R3 (2026-10-02), detailed by ADR-010/011

## Context
All extensions must be discoverable without reading arbitrary binary data as text or retaining unbounded content.

## Decision
R1 scans metadata only, including regular files of every extension and size, symlink entries and special-file metadata. It never follows a symlink for content and never opens a FIFO/device as content. `max_diff_bytes` is a positive int64 byte budget, provisional default 5 MiB; it does not filter the scan. R3 classifies via bounded reads and UTF-8/content evidence, not extension, storing the result in the captured Ref. TooLarge/Binary remain visible. Snapshot cache must be private and outside the monitored root.

## Alternatives
Extension allowlists miss files. Reading all content during scan violates resource and privacy goals.

## Consequences
R3 replaces R2's private eligibility probe with the shared Classifier/Probe and separates max_snapshot_bytes (default8MiB) from max_diff_bytes (default5MiB), with max_diff_lines and computation bounds. Non-text, oversized or over-budget contents retain metadata/hash only. GetDiff skips Binary/Unsupported/TooLarge and reports Unavailable when before content was not retained. Limits remain provisional pending P12; they are not performance guarantees. Paths are escaped by CLI rendering; UI must not render arbitrary bytes as text.

## Migration
Changes to limits or binary policy require tests and ADR updates; do not silently turn max_diff_bytes into a scan exclusion.
