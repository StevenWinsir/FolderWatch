import { describe, expect, it } from 'vitest';
import { displayLines, displayText } from '../src/diff-lines';
import type { DiffResult } from '../src/ipc';

const diff: DiffResult = {
  sessionId: 'session', path: 'a.txt', kind: 'modified', status: 'text', reason: '', generation: '1', version: '1',
  hunks: [{
    oldStart: 7, oldLines: 6, newStart: 7, newLines: 6,
    lines: [
      { kind: 'context', oldLine: 7, newLine: 7, text: 'line-7', noNewline: false },
      { kind: 'context', oldLine: 8, newLine: 8, text: 'line-8', noNewline: false },
      { kind: 'context', oldLine: 9, newLine: 9, text: 'line-9', noNewline: false },
      { kind: 'removed', oldLine: 10, newLine: 0, text: 'line-10', noNewline: false },
      { kind: 'added', oldLine: 0, newLine: 10, text: 'changed-10', noNewline: false },
      { kind: 'context', oldLine: 11, newLine: 11, text: 'line-11', noNewline: false },
      { kind: 'context', oldLine: 12, newLine: 12, text: 'line-12', noNewline: false },
    ],
  }],
};

describe('diff display line mapping', () => {
  it('keeps the backend line numbers while omitting the opposite side', () => {
    const modified = displayLines('modified', diff);
    expect(modified.map(line => line.line)).toEqual([7, 8, 9, 10, 11, 12]);
    expect(displayText(modified)).toContain('changed-10');
    expect(displayLines('original', diff).map(line => line.line)).toEqual([7, 8, 9, 10, 11, 12]);
  });
});
