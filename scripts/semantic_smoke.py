#!/usr/bin/env python3
"""R3 semantic-stream black-box checks on isolated temporary files only."""
from __future__ import annotations
import json
import os
from pathlib import Path
import queue
import signal
import subprocess
import sys
import tempfile
import threading
import time


def main() -> None:
    binary = Path(sys.argv[1]).resolve(strict=True)
    checks = 0
    with tempfile.TemporaryDirectory(prefix="folderwatch-r3-") as tmp:
        base = Path(tmp)
        root, home, cache = (base / n for n in ("项目 with spaces", "home", "cache"))
        for p in (root, home, cache):
            p.mkdir()
        original = "baseline secret content\n"
        (root / "base.txt").write_text(original, encoding="utf-8")
        (root / ".folderwatchignore").write_text("*.tmp\n", encoding="utf-8")
        env = dict(os.environ, HOME=str(home), XDG_CONFIG_HOME=str(home),
                   TMPDIR=str(cache), TMP=str(cache), TEMP=str(cache))
        proc = subprocess.Popen([str(binary), "--watch", "--json", "--debounce=30ms",
                                 "--max-pending-events=1", "--max-snapshot-bytes=32B", str(root)],
                                env=env, cwd=root, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        messages: queue.Queue = queue.Queue()
        transcript: list[str] = []
        state: dict[str, dict] = {}
        sequence = 0
        def reader() -> None:
            assert proc.stdout is not None
            for line in proc.stdout:
                transcript.append(line)
                try:
                    messages.put(json.loads(line))
                except Exception as exc:
                    messages.put(exc)
            messages.put(None)
        thread = threading.Thread(target=reader)
        thread.start()
        def receive(timeout: float):
            nonlocal sequence
            event = messages.get(timeout=timeout)
            assert isinstance(event, dict), event
            assert event["sequence"] > sequence
            sequence = event["sequence"]
            assert "paths" not in event, "CLI leaked internal path invalidations"
            if "state" in event:
                state.clear()
                state.update((c["path"], c) for c in event["state"]["changes"])
            else:
                batch = event.get("batch", {})
                assert not batch.get("reload"), "reload omitted authoritative state"
                for name in batch.get("removed", []):
                    state.pop(name, None)
                for c in batch.get("upserts", []):
                    state[c["path"]] = c
            return event
        def await_state(expected: dict[str, str]) -> None:
            nonlocal checks
            deadline = time.monotonic() + 8
            # At least one event must follow each filesystem operation.
            while True:
                receive(max(0.01, deadline - time.monotonic()))
                if {p: c["kind"] for p, c in state.items()} == expected:
                    checks += 1
                    return
                assert time.monotonic() < deadline, (state, expected)
        try:
            ready = receive(8)
            assert ready["type"] == "ready" and ready["generation"] == 1 and not state
            checks += 1
            target = root / "base.txt"
            target.write_text("edited\n", encoding="utf-8")
            await_state({"base.txt": "modified"})
            (root / "save.tmp").write_text(original, encoding="utf-8")
            (root / "save.tmp").replace(target)
            await_state({})
            (root / "new.txt").write_text("new\n", encoding="utf-8")
            await_state({"new.txt": "added"})
            (root / "new.txt").unlink()
            await_state({})
            target.unlink()
            await_state({"base.txt": "deleted"})
            target.write_text(original, encoding="utf-8")
            await_state({})
            target.rename(root / "renamed.txt")
            expected = {"base.txt": "deleted", "renamed.txt": "added"}
            await_state(expected)
            (root / "image.unknown").write_bytes(b"a\x00b")
            expected["image.unknown"] = "added"
            await_state(expected)
            assert state["image.unknown"]["after"]["classification"]["kind"] == "binary"
            checks += 1
            (root / "utf16.txt").write_bytes(b"\xff\xfea\x00")
            expected["utf16.txt"] = "added"
            await_state(expected)
            assert state["utf16.txt"]["after"]["classification"]["kind"] == "unsupported-text"
            checks += 1
            (root / "large.txt").write_bytes(b"x" * 64)
            expected["large.txt"] = "added"
            await_state(expected)
            assert state["large.txt"]["after"]["classification"]["kind"] == "too-large"
            checks += 1
            assert original.strip() not in "".join(transcript)
            checks += 1
            proc.send_signal(signal.SIGINT)
            assert proc.wait(timeout=8) == 130
            thread.join(timeout=3)
            assert not thread.is_alive() and not list(cache.glob("folderwatch-*"))
            checks += 1
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait(timeout=5)
            thread.join(timeout=3)
            assert proc.stdout is not None and proc.stderr is not None
            errors = proc.stderr.read()
            proc.stdout.close()
            proc.stderr.close()
            if proc.returncode not in (0, 130):
                print(errors, file=sys.stderr)
    print(f"R3 semantic smoke: {checks} checks passed")


if __name__ == "__main__":
    main()
