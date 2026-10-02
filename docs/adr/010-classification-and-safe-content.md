# ADR-010: Captured classification and independent byte limits

Status: Accepted, implemented in R3 (2026-10-02)

## Context
R2 retained snapshots using a private UTF-8 eligibility probe. R3 must distinguish safe text, binary, recognized unsupported text and oversized files without changing classification later by rereading today's contents in place of the baseline. Extension allowlists would lose arbitrary file types.

## Decision
`internal/filetype.Classifier` is a UI-independent bounded-reader interface. `StreamClassifier` checks a known size before reading: above the classification cap it returns `too-large` without calling the reader. Otherwise it reads in 32 KiB chunks, rejects growth/truncation, and cooperatively checks context. A shared incremental `Probe` validates the complete bounded stream and carries at most three incomplete UTF-8 bytes plus four prefix bytes for BOM recognition.

Statuses are `text` (UTF-8, including empty files and UTF-8 BOM), `binary` (invalid UTF-8, NUL, C0 controls except LF/CR/TAB, DEL or C1), `unsupported-text` (UTF-16/32 BOM), `too-large`, and `unsupported` for non-regular entries. BOM-less non-UTF-8 data is not decoded or guessed: it is binary. Filename extensions never participate. Oversize status takes precedence over encoding.

Snapshot capture uses this same Probe and stores the immutable, comparable classification in `snapshot.Ref.Class` alongside the hash of exactly those captured bytes. The old independent private probe is removed. Non-text provisional spools are discarded; binary/unsupported content is never exposed as text. Symlinks remain link metadata/target-string hash, not target content; special files are metadata-only. Unix safe regular-file opens and identity/stability checks remain in the existing snapshot layer.

Separate configuration limits: `max_snapshot_bytes` defaults to **8 MiB** and caps classification/retention per file; `max_diff_bytes` defaults to **5 MiB** and caps each diff input. Each is validated in 1..64 MiB. A lower diff cap does not change the file's classification or discovery. Oversized regular files still receive streaming SHA-256 for reliable content comparison, but skip text probing; that hash I/O is independent of classification and remains cancellable between blocks.

## Alternatives
Keeping a second classifier beside the R2 probe would create divergent safety decisions. Prefix-only text acceptance can miss late binary bytes. Decoding arbitrary encodings risks incorrect rendering and is beyond v1. Reusing only max_diff_bytes would couple retention/classification to presentation limits.

## Consequences
Classification memory does not grow with file size. A generic blocking io.Reader cannot be forcibly interrupted inside its Read call; production reads use the existing safe local-file layer, never FIFO/device content. Huge-file hashing can still take I/O time. CRLF and missing-final-newline bytes are preserved. UI adapters must render structured text safely and escape path/control presentation; no text is inserted into a shell or uploaded.

## Migration
R2's single-file retention cap is replaced by the new independent default 8 MiB. Existing max_diff_bytes keeps its 5 MiB default but now means an actual diff limit. Applications requiring the earlier retention cap can explicitly set max_snapshot_bytes='5MiB'. Snapshot memory/disk totals and normal scan behavior remain unchanged. Changing supported encodings or these provisional defaults requires regression tests and later R5 measurement.
