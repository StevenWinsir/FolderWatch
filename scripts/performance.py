#!/usr/bin/env python3
"""Run isolated synthetic workloads; keep raw evidence, never invent PASS budgets."""
import argparse
import json
import os
from pathlib import Path
import platform
import plistlib
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def command(args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "artifacts/performance-v1")
    parser.add_argument("--idle-seconds", type=int, default=10)
    parser.add_argument("--samples", type=int, default=20)
    args = parser.parse_args()
    if not 1 <= args.idle_seconds <= 600 or not 1 <= args.samples <= 1000:
        parser.error("idle-seconds must be 1..600 and samples 1..1000")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, GOFLAGS="-mod=vendor", GOWORK="off", CGO_ENABLED="0")
    subprocess.run(["go", "build", "-trimpath", "-o", "bin/fwbench", "./cmd/fwbench"], cwd=ROOT, env=env, check=True)
    metadata = {"platform": platform.platform(), "go": command(["go", "version"]),
                "commit": command(["git", "rev-parse", "HEAD"]),
                "dirty": bool(command(["git", "status", "--porcelain"])),
                "filesystem": "not observed", "notes": "Synthetic fixtures; warm local filesystem; no race instrumentation. RSS is process high-water mark, including fixture generation. Latency ends at authoritative Core state, not rendered pixels."}
    if sys.platform == "darwin":
        metadata.update(cpu=command(["sysctl", "-n", "machdep.cpu.brand_string"]),
                        ram_bytes=int(command(["sysctl", "-n", "hw.memsize"])),
                        macos=command(["sw_vers", "-productVersion"]),
                        filesystem=plistlib.loads(subprocess.check_output(["diskutil", "info", "-plist", "/"]))["FilesystemType"],
                        storage=plistlib.loads(subprocess.check_output(["diskutil", "info", "-plist", "/"]))["SolidState"])
    (output / "environment.json").write_text(json.dumps(metadata, indent=2) + "\n")
    workloads = [
        ("1k", ["--files", "1000", "--burst", "100"]),
        ("10k", ["--files", "10000", "--burst", "100", "--cpu-profile", str(output / "10k.cpu.pprof"), "--heap-profile", str(output / "10k.heap.pprof"), "--goroutine-profile", str(output / "10k.goroutines.txt")]),
        ("text-5MiB", ["--files", "1", "--bytes", str(5 << 20), "--burst", "1"]),
        ("text-10MiB", ["--files", "1", "--bytes", str(10 << 20), "--burst", "1"]),
        ("binary-64MiB", ["--files", "1", "--bytes", str(64 << 20), "--burst", "1", "--binary"]),
        ("depth-32", ["--files", "100", "--depth", "32", "--burst", "100"]),
        ("lifecycle-50", ["--files", "10", "--burst", "10", "--cycles", "50"]),
    ]
    reports = {}
    for name, flags in workloads:
        print("Measuring " + name, flush=True)
        result = subprocess.run([str(ROOT / "bin/fwbench"), "--samples", str(args.samples), "--idle", str(args.idle_seconds) + "s"] + flags,
                                cwd=ROOT, text=True, capture_output=True, timeout=1800)
        (output / (name + ".stderr.txt")).write_text(result.stderr)
        (output / (name + ".json")).write_text(result.stdout)
        if result.returncode:
            raise RuntimeError(name + ": " + result.stderr)
        report = json.loads(result.stdout)
        if not report["correctness_and_cleanup_passed"]:
            raise RuntimeError(name + " failed correctness/cleanup")
        reports[name] = report
        print(json.dumps({"workload": name, "ready_ms": report["prepare_and_ready_ms"], "heap_bytes": report["settled_heap_bytes"], "idle_cpu_percent": report["idle_cpu_percent_one_core"], "p95_ms": report["p95_write_complete_to_core_state_ms"], "fds_after": report["fds_after"]}), flush=True)
    (output / "summary.json").write_text(json.dumps(reports, indent=2) + "\n")
    for kind in ("cpu", "heap"):
        result = subprocess.run(["go", "tool", "pprof", "-top", str(ROOT / "bin/fwbench"), str(output / ("10k." + kind + ".pprof"))], cwd=ROOT, text=True, capture_output=True, check=True)
        (output / ("10k." + kind + ".top.txt")).write_text(result.stdout)
    print("All workloads completed: " + str(output))


if __name__ == "__main__":
    main()
