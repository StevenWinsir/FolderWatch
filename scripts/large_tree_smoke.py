#!/usr/bin/env python3
"""Real macOS CLI coverage with a 64-fd hard limit, including kqueue failover.

Usage: python3 scripts/large_tree_smoke.py NATIVE_BINARY CGO_DISABLED_BINARY
All fixtures, configuration and cache live in an owned temporary directory.
Only child processes receive the restrictive resource limit.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import queue
import resource
import signal
import subprocess
import sys
import tempfile
import threading
import time


def restrict_descriptors() -> None:
    resource.setrlimit(resource.RLIMIT_NOFILE, (64, 64))


def exercise(binary: Path, expect_fallback: bool) -> dict:
    with tempfile.TemporaryDirectory(prefix="folderwatch-low-fd-") as temporary:
        parent = Path(temporary)
        root, home, cache = (parent / name for name in ("root", "home", "cache"))
        for path in (root, home, cache):
            path.mkdir()
        for number in range(512):
            (root / f"f-{number:04d}.txt").write_text("baseline\n", encoding="utf-8")
        env = dict(os.environ, HOME=str(home), XDG_CONFIG_HOME=str(home),
                   TMPDIR=str(cache), TMP=str(cache), TEMP=str(cache))
        process = subprocess.Popen(
            [str(binary.resolve()), "--watch", "--json", "--debounce=30ms", str(root)],
            cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=True, preexec_fn=restrict_descriptors,
        )
        messages: queue.Queue = queue.Queue()
        errors: list[str] = []
        state: dict[str, dict] = {}
        warnings: list[str] = []

        def reader() -> None:
            assert process.stdout is not None
            for line in process.stdout:
                try:
                    messages.put(json.loads(line))
                except Exception as error:
                    messages.put(error)
            messages.put(None)

        def error_reader() -> None:
            assert process.stderr is not None
            for line in process.stderr:
                if len(errors) < 64:
                    errors.append(line)

        output_thread = threading.Thread(target=reader, daemon=True)
        error_thread = threading.Thread(target=error_reader, daemon=True)
        output_thread.start()
        error_thread.start()

        def receive(timeout: float) -> dict:
            event = messages.get(timeout=max(0.01, timeout))
            if not isinstance(event, dict):
                raise AssertionError(f"watch pipeline ended: {event!r}; {''.join(errors)}")
            if "state" in event:
                state.clear()
                state.update((item["path"], item) for item in event["state"]["changes"])
            else:
                batch = event.get("batch", {})
                assert not batch.get("reload"), "reload without authoritative state"
                for name in batch.get("removed", []):
                    state.pop(name, None)
                for item in batch.get("upserts", []):
                    state[item["path"]] = item
            if event.get("type") == "warning":
                warnings.append(event.get("message", ""))
            return event

        def await_change(path: str, kind: str, content: str = "") -> None:
            expected_hash = hashlib.sha256(content.encode()).hexdigest()
            deadline = time.monotonic() + 20
            while True:
                receive(deadline - time.monotonic())
                item = state.get(path, {})
                if item.get("kind") == kind and (kind == "deleted" or item.get("after", {}).get("hash") == expected_hash):
                    return
                assert time.monotonic() < deadline, (path, kind, state)

        try:
            ready = receive(20)
            assert ready["type"] == "ready" and ready["baseline_files"] == 513, ready
            if expect_fallback:
                deadline = time.monotonic() + 20
                while not any("monitoring continues using periodic metadata scans" in message for message in warnings):
                    receive(deadline - time.monotonic())
                    assert time.monotonic() < deadline, "missing fallback warning"
                assert any("too many open files" in message.lower() for message in warnings), warnings
            target = root / "f-0511.txt"
            target.write_text("changed\n", encoding="utf-8")
            await_change(target.name, "modified", "changed\n")
            replacement = root / "save.tmp"
            replacement.write_text("atomic\n", encoding="utf-8")
            replacement.replace(target)
            await_change(target.name, "modified", "atomic\n")
            nested = root / "new" / "deep"
            nested.mkdir(parents=True)
            (nested / "late.txt").write_text("new subtree\n", encoding="utf-8")
            await_change("new/deep/late.txt", "added", "new subtree\n")
            target.unlink()
            await_change(target.name, "deleted")
            if not expect_fallback:
                assert not any("monitoring continues using periodic metadata scans" in message for message in warnings), "native probe must use a CGO-enabled FSEvents build"
            assert process.poll() is None, errors
            process.send_signal(signal.SIGINT)
            assert process.wait(timeout=10) == 130, errors
            assert not list(cache.glob("folderwatch-*")), "private cache leaked after shutdown"
            return {"binary": binary.name, "hard_fd_limit": 64, "baseline_files": 513,
                    "fallback": expect_fallback, "checks": ["ready", "modify", "atomic-save", "new-subtree", "delete", "cleanup"],
                    "warnings": warnings}
        finally:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=5)
            output_thread.join(timeout=2)
            error_thread.join(timeout=2)
            if process.stdout is not None:
                process.stdout.close()
            if process.stderr is not None:
                process.stderr.close()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("native_binary", type=Path)
    parser.add_argument("nocgo_binary", type=Path)
    args = parser.parse_args()
    if sys.platform != "darwin":
        print("SKIP: this probe specifically exercises macOS FSEvents and kqueue fd accounting")
        return
    for binary in (args.native_binary, args.nocgo_binary):
        if not binary.is_file():
            parser.error(f"binary not found: {binary}")
    results = [exercise(args.native_binary, False), exercise(args.nocgo_binary, True)]
    print(json.dumps({"status": "PASS", "results": results}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
