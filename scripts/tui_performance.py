#!/usr/bin/env python3
"""Measure write completion to fresh real-binary TUI output on a PTY, not screen pixels."""
import argparse
import json
import os
from pathlib import Path
import tempfile
import time

from tui_smoke import Terminal


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--files", type=int, default=10000)
    parser.add_argument("--samples", type=int, default=20)
    parser.add_argument("--output", type=Path, default=Path("artifacts/tui-performance.json"))
    args = parser.parse_args()
    if not 100 <= args.files <= 50000 or not 2 <= args.samples <= 1000:
        parser.error("files must be 100..50000 and samples 2..1000")
    if args.output.exists():
        parser.error("output exists; refusing to overwrite evidence")
    baseline = "b" * 1023 + "\n"
    edited = "EDITED" + baseline[6:]
    with tempfile.TemporaryDirectory(prefix="folderwatch-tui-bench-") as temp:
        base = Path(temp)
        root, home, cache = (base/name for name in ("项目 with spaces", "home", "cache"))
        for path in (root, home, cache):
            path.mkdir()
        paths = [root/("f%05d.txt" % index) for index in range(args.files)]
        for path in paths:
            path.write_text(baseline)
        env = dict(os.environ, HOME=str(home), XDG_CONFIG_HOME=str(home), TERM="xterm-256color", NO_COLOR="1", TMPDIR=str(cache), TMP=str(cache), TEMP=str(cache))
        for key in ("CI", "CLICOLOR", "CLICOLOR_FORCE"):
            env.pop(key, None)
        term = Terminal(args.binary.resolve(strict=True), root, env, ["--no-mouse", "--debounce=150ms"])
        samples = []
        try:
            term.wait("Monitoring", timeout=30)
            term.wait("0 files changed", timeout=30)
            mark = term.send(b"p")
            term.wait("Paused", mark, timeout=30)
            mark = term.send(b"p")
            term.wait("Monitoring", mark, timeout=30)
            for index in range(args.samples):
                term.pump(0.02)
                mark = len(term.data)
                content, expected = (edited, "100 files changed") if index % 2 == 0 else (baseline, "0 files changed")
                for path in paths[:100]:
                    path.write_text(content)
                written = time.monotonic()
                deadline = written + 10
                while expected not in term.text(mark):
                    term.pump(0.002)
                    if term.proc.poll() is not None or time.monotonic() > deadline:
                        raise AssertionError("TUI failed to publish fresh exact change count: " + term.text(mark)[-2000:])
                samples.append((time.monotonic()-written)*1000)
            term.finish(b"q", 0)
            if list(cache.iterdir()):
                raise AssertionError("TUI performance run leaked cache")
        finally:
            term.close()
        ordered = sorted(samples)
        p95 = ordered[(95*len(ordered)+99)//100-1]
        report = {"files": args.files, "bytes_per_file": 1024, "burst": 100, "samples": len(samples),
                  "viewport": "120x36", "debounce_ms": 150, "poll_resolution_ms": 2,
                  "write_complete_to_tui_output_ms": samples, "p95_ms": p95,
                  "p95_minus_nominal_debounce_ms": max(0, p95-150),
                  "correctness_and_cleanup_passed": True,
                  "measurement_boundary": "last write return -> fresh changed-count bytes from real TUI on PTY; alternating 100 modifications / full restoration; not compositor latency or Terminal.app/iTerm2 human QA"}
        args.output.parent.mkdir(parents=True, exist_ok=True)
        with args.output.open("x") as output:
            json.dump(report, output, indent=2)
            output.write("\n")
        print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
