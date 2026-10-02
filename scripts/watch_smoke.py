#!/usr/bin/env python3
"""R2 monitoring regressions through the R3 semantic stream, using only isolated temporary fixtures."""
from __future__ import annotations

import json
import os
from pathlib import Path
import queue
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time


def main() -> None:
    binary = Path(sys.argv[1]).resolve()
    checks = 0
    with tempfile.TemporaryDirectory(prefix="folderwatch-r2-smoke-") as temp:
        base = Path(temp)
        root, home, cache = (base / name for name in ("目录 with spaces", "home", "cache"))
        for directory in (root, home, cache):
            directory.mkdir()
        target = root / "a.txt"
        target.write_text("DO-NOT-PRINT-FILE-CONTENT\n", encoding="utf-8")
        (root / ".folderwatchignore").write_text("*.tmp\nignored/\n", encoding="utf-8")
        env = dict(os.environ, HOME=str(home), XDG_CONFIG_HOME=str(home), TMPDIR=str(cache), TMP=str(cache), TEMP=str(cache))
        proc = subprocess.Popen(
            [str(binary), "--watch", "--json", "--debounce=60ms", "--max-pending-events=8", str(root)],
            cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
        )
        messages: queue.Queue[object] = queue.Queue()
        transcript: list[str] = []

        def read_output() -> None:
            assert proc.stdout is not None
            for line in proc.stdout:
                transcript.append(line)
                try:
                    messages.put(json.loads(line))
                except json.JSONDecodeError as exc:
                    messages.put(exc)
            messages.put(None)

        reader = threading.Thread(target=read_output, name="watch-smoke-reader")
        reader.start()

        def event(predicate, timeout: float = 8.0):
            end = time.monotonic() + timeout
            while time.monotonic() < end:
                value = messages.get(timeout=max(0.01, end - time.monotonic()))
                if not isinstance(value, dict):
                    raise AssertionError(f"invalid/closed event stream: {value!r}")
                if predicate(value):
                    return value
            raise AssertionError("event timeout")

        def invalidated(path: str):
            return event(lambda e: e.get("type") == "changes" and
                         (e.get("reconcile") or path in e.get("batch", {}).get("removed", []) or
                          any(p["path"] == path for p in e.get("batch", {}).get("upserts", []))))

        try:
            ready = event(lambda e: e.get("type") == "ready")
            assert ready["generation"] == 1 and ready["reconcile"]
            checks += 1
            assert sorted(p.name for p in root.iterdir()) == [".folderwatchignore", "a.txt"]
            checks += 1
            owned = list(cache.glob("folderwatch-*"))
            assert len(owned) == 3 and all((p.stat().st_mode & 0o777) == 0o700 for p in owned)
            checks += 1

            # Direct-write and replacement models, not a claim of running VS Code.
            for text in ("direct write", "second write", "restored model"):
                target.write_text(text, encoding="utf-8")
                assert invalidated("a.txt")["generation"] == 1
                checks += 1
            replacement = root / "save.tmp"
            replacement.write_text("atomic replacement", encoding="utf-8")
            replacement.replace(target)
            assert invalidated("a.txt")["generation"] == 1
            checks += 1

            sub = root / "new" / "deep"
            sub.mkdir(parents=True)
            (sub / "immediate").write_text("present before registration", encoding="utf-8")
            invalidated("new/deep/immediate")
            checks += 1
            (sub / "later").write_text("after registration", encoding="utf-8")
            invalidated("new/deep/later")
            checks += 1
            for i in range(30):
                target.write_text(f"continuous save {i}", encoding="utf-8")
            invalidated("a.txt")
            checks += 1

            vim = shutil.which("vim")
            if vim is not None:
                for backupcopy in ("yes", "no"):
                    subprocess.run(
                        [vim, "-Nu", "NONE", "-n", "-es", str(target),
                         "-c", f"set backupcopy={backupcopy} nobackup writebackup",
                         "-c", f'call setline(1, "vim backupcopy={backupcopy}")', "-c", "wq"],
                        cwd=root, env=env, timeout=10, check=True, capture_output=True, text=True,
                    )
                    assert target.read_text(encoding="utf-8").startswith(f"vim backupcopy={backupcopy}")
                    assert invalidated("a.txt")["generation"] == 1
                    checks += 1
                print("Vim actual headless saves: backupcopy=yes/no PASS")
            else:
                print("Vim actual editor check SKIPPED: executable not installed")

            target.unlink()
            invalidated("a.txt")
            checks += 1
            renamed = root / "renamed"
            (sub / "later").rename(renamed)
            invalidated("renamed")
            checks += 1
            assert "DO-NOT-PRINT-FILE-CONTENT" not in "".join(transcript)
            checks += 1

            proc.send_signal(signal.SIGINT)
            assert proc.wait(timeout=8) == 130
            checks += 1
            reader.join(timeout=3)
            assert not reader.is_alive()
            assert not list(cache.glob("folderwatch-*"))
            checks += 1
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait(timeout=5)
            reader.join(timeout=3)
            assert proc.stdout is not None and proc.stderr is not None
            stderr = proc.stderr.read()
            proc.stdout.close()
            proc.stderr.close()
            if proc.returncode not in (130, 0):
                print(f"watch exit={proc.returncode}, stderr={stderr!r}", file=sys.stderr)
    print(f"R2 watch smoke: {checks} checks passed; direct-write and atomic-save models verified")


if __name__ == "__main__":
    main()
