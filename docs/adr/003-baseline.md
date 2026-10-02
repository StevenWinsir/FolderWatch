# ADR-003: Session baseline semantics

Status: Accepted design; implementation deferred to R2/P5 (2026-10-01)

## Context
Users want to compare current contents with the start of a session, not the previous save.

## Decision
A session's initial capture is the baseline. Ordinary writes never advance it. Reset is an application-service operation: capture a stable current state, replace the baseline, clear semantic changes, reconcile queued events. Initial scan metadata is not a content baseline.

## Alternatives
Comparing with the immediately previous save loses the session's intended meaning. Letting the UI reset files individually breaks atomic semantics.

## Consequences
R1 returns only metadata and does not hash, capture contents, reset or publish changes. R2 must implement bounded snapshot ownership and cancellation, and test start/edit/re-edit/restore/reset.

## Migration
Keep R1 inventory types separate from SnapshotRef and semantic FileChange, which will be introduced when their actual lifecycles exist.
