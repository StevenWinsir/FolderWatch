# ADR-003: Session baseline semantics

Status: Accepted; snapshot/baseline core implemented in R2/P5 (2026-10-02)

## Context
Users want to compare current contents with the start of a session, not the previous save.

## Decision
A session's initial capture is the baseline. Ordinary writes never advance it. Reset is an application-service operation: capture a stable current state, replace the baseline, clear semantic changes, reconcile queued events. Initial scan metadata is not a content baseline.

## Alternatives
Comparing with the immediately previous save loses the session's intended meaning. Letting the UI reset files individually breaks atomic semantics.

## Consequences
R1 scan still returns metadata only. R2 implements bounded snapshot ownership, stable before contents/hash, cancellation, generation replacement and reset rollback; details are in ADR-008. R3 now integrates atomic semantic-list clearing after successful Reset, while failed/cancelled resets preserve the old list. See ADR-012; this does not imply a TUI reset control exists.

## Migration
R2 snapshot.Ref remains distinct from R1 FileMeta. R3 exposes changes.Summary/Batch and respects generation/stale-reference boundaries; do not auto-advance baseline on ordinary saves.
