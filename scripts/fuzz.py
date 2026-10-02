#!/usr/bin/env python3
"""Bounded fuzz matrix with retained logs and nonzero failure propagation."""
import argparse
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
TARGETS = [("config", "FuzzSize"), ("pathutil", "FuzzKeyContainment"),
           ("eventnorm", "FuzzNormalize"), ("filetype", "FuzzClassifier"),
           ("snapshot", "FuzzTextProbeChunkBoundaries"), ("diff", "FuzzDiffReconstruct")]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--seconds", type=int, default=10)
    parser.add_argument("--output", type=Path, default=ROOT / "artifacts/fuzz")
    args = parser.parse_args()
    if not 1 <= args.seconds <= 300:
        parser.error("seconds must be 1..300")
    args.output.mkdir(parents=True, exist_ok=True)
    for package, target in TARGETS:
        result = subprocess.run(["go", "test", "./internal/" + package, "-run=^$", "-fuzz=^" + target + "$", "-fuzztime=" + str(args.seconds) + "s", "-parallel=2"],
                                cwd=ROOT, env=dict(os.environ, GOMAXPROCS="2", GOFLAGS="-mod=vendor", GOWORK="off"),
                                text=True, capture_output=True, timeout=args.seconds + 180)
        output = result.stdout + result.stderr
        (args.output / (target + ".log")).write_text(output)
        print(output, flush=True)
        if result.returncode:
            raise SystemExit(result.returncode)


if __name__ == "__main__":
    main()
