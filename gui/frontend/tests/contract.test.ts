import { expect, it } from 'vitest';
import fixture from './fixtures/ipc-v1.json';
import { eventNames, isSession, type CoreEvent } from '../src/ipc';

it('consumes the exact Go-verified protocol fixture without uint64 precision loss', () => {
  const event: CoreEvent = fixture;
  expect(event.protocol).toBe(1);
  expect(eventNames).toContain(event.name);
  expect(isSession(event.status)).toBe(true);
  expect(BigInt(event.status.sequence) + 1n).toBe(BigInt(event.status.generation));
  expect(BigInt(event.status.generation) + 1n).toBe(BigInt(event.status.version));
  expect(event.status.root).toBe('/fixture/世界');
  expect(Object.keys(event).sort()).toEqual(['clientId', 'name', 'protocol', 'reload', 'status']);
});
