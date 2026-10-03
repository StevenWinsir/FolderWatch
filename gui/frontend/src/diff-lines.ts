import type { DiffLine, DiffResult } from './ipc';

export type DiffSide = 'original' | 'modified';

export interface DisplayLine {
  text: string;
  line: number;
}

const kinds: Record<DiffSide, ReadonlySet<DiffLine['kind']>> = {
  original: new Set(['context', 'removed']),
  modified: new Set(['context', 'added']),
};

export function displayLines(side: DiffSide, diff: DiffResult): DisplayLine[] {
  const lineNumber = side === 'original' ? 'oldLine' : 'newLine';
  return diff.hunks.flatMap(hunk => hunk.lines
    .filter(line => kinds[side].has(line.kind))
    .map(line => ({ text: line.text, line: line[lineNumber] })));
}

export function displayText(lines: DisplayLine[]): string {
  return lines.map(line => line.text).join('\n');
}
