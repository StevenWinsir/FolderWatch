# ADR-004: Text, binary and large files

Status: Accepted design; classifier/diff implementation deferred to R3 (2026-10-01)

## Context
All extensions must be discoverable without reading arbitrary binary data as text or retaining unbounded content.

## Decision
R1 scans metadata only, including regular files of every extension and size, symlink entries and special-file metadata. It never follows a symlink for content and never opens a FIFO/device as content. `max_diff_bytes` is a positive int64 byte budget, provisional default 5 MiB; it does not filter the scan. R3 will classify via bounded reads and UTF-8/content evidence, not extension. TooLarge/Binary remain visible. Snapshot cache must be private and outside the monitored root.

## Alternatives
Extension allowlists miss files. Reading all content during scan violates resource and privacy goals.

## Consequences
R2 uses max_diff_bytes as the per-file retained snapshot content cap. Binary/invalid UTF-8, oversized and over-budget contents retain metadata/hash only; this storage-eligibility probe is not R3's Classifier API (see ADR-008). The flag still has no diff effect until R3. Snapshot/probe/line/worker limits are added with the implementing stages and measured in P12; the current 5 MiB default is not a measured performance guarantee. Control characters in paths must be escaped by CLI rendering.

## Migration
Changes to limits or binary policy require tests and ADR updates; do not silently turn max_diff_bytes into a scan exclusion.
