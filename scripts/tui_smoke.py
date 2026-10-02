#!/usr/bin/env python3
"""R4 real-binary PTY acceptance. Standard library only; never touches user files.

This drives a pseudo-terminal, not Terminal.app/iTerm2 manual visual acceptance.
Assertions read only output emitted after each action to avoid stale matches.
"""
from __future__ import annotations

import errno
import fcntl
import json
import os
from pathlib import Path
import pty
import re
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time

CSI = re.compile(rb"\x1b\[[0-?]*[ -/]*[@-~]")
OSC = re.compile(rb"\x1b\][^\x07]*(?:\x07|\x1b\\)")


class Terminal:
    def __init__(self, binary: Path, root: Path, env: dict[str, str], flags: list[str]):
        self.master, self.slave = pty.openpty()
        self.before = termios.tcgetattr(self.slave)
        self.data = bytearray()
        self.resize(120, 36, notify=False)
        self.proc = subprocess.Popen(
            [str(binary), "--debounce=30ms", *flags, str(root)],
            cwd=root, env=env, stdin=self.slave, stdout=self.slave, stderr=self.slave,
        )

    def resize(self, width: int, height: int, notify: bool = True) -> int:
        mark = len(self.data)
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
        if notify:
            self.proc.send_signal(signal.SIGWINCH)
        return mark

    def pump(self, seconds: float = 0.05) -> None:
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            ready, _, _ = select.select([self.master], [], [], max(0, end-time.monotonic()))
            if not ready:
                continue
            try:
                chunk = os.read(self.master, 65536)
            except OSError as exc:
                if exc.errno == errno.EIO:
                    return
                raise
            if not chunk:
                return
            self.data.extend(chunk)
            if len(self.data) > 8 << 20:
                raise AssertionError("PTY transcript exceeded bounded test budget")

    def text(self, mark: int = 0) -> str:
        data = CSI.sub(b"", OSC.sub(b"", bytes(self.data[mark:])))
        return data.decode("utf-8", "replace").replace("\r", "")

    def wait(self, text: str, mark: int = 0, timeout: float = 10) -> None:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.pump()
            if text in self.text(mark):
                return
            if self.proc.poll() is not None:
                break
        raise AssertionError(f"missing {text!r}; exit={self.proc.poll()}; recent={self.text(mark)[-5000:]!r}")

    def send(self, keys: bytes) -> int:
        self.pump(0.1)
        mark = len(self.data)
        os.write(self.master, keys)
        return mark

    def search(self, query: bytes) -> int:
        # Enter filter mode before delivering the query. Bubble Tea deliberately
        # groups a pasted rune run; pasting '/query' in command mode is not a
        # sequence of command keystrokes and must not accidentally execute q/r.
        mark = self.send(b"/")
        self.send(query + b"\r")
        return mark

    def finish(self, key: bytes | None, expected: int) -> None:
        if key is not None:
            self.send(key)
        deadline = time.monotonic()+10
        while self.proc.poll() is None and time.monotonic() < deadline:
            self.pump()
        assert self.proc.poll() == expected, (self.proc.poll(), self.text()[-3000:])
        self.pump(0.1)
        after = termios.tcgetattr(self.slave)
        before = list(self.before)
        # Darwin sets the kernel-managed PENDIN bit when canonical mode is
        # restored. It is not a user mode; every other attribute must match.
        pendin = getattr(termios, "PENDIN", 0)
        after[3] &= ~pendin
        before[3] &= ~pendin
        assert after == before, ("terminal attributes not restored", before, after)
        assert b"\x1b[?1049l" in self.data, "alternate screen not restored"
        assert b"\x1b[?25h" in self.data, "cursor not restored"

    def close(self) -> None:
        if self.proc.poll() is None:
            self.proc.kill()
            self.proc.wait(timeout=5)
        os.close(self.master)
        os.close(self.slave)


def main() -> None:
    binary = Path(sys.argv[1]).resolve()
    checks = 0
    with tempfile.TemporaryDirectory(prefix="folderwatch-r4-pty-") as temp:
        base = Path(temp)
        root, home, cache = [base/name for name in ("项目 with spaces", "home", "cache")]
        for path in (root, home, cache):
            path.mkdir()
        env = dict(os.environ, HOME=str(home), XDG_CONFIG_HOME=str(home), TERM="xterm-256color",
                   NO_COLOR="1", TMPDIR=str(cache), TMP=str(cache), TEMP=str(cache))
        # This child represents an interactive terminal, not the CI log stream.
        # termenv intentionally disables automatic color detection when CI is
        # nonempty. Isolate inherited color preferences as well; NO_COLOR above
        # and its removal below explicitly select the two tested profiles.
        for key in ("CI", "CLICOLOR", "CLICOLOR_FORCE"):
            env.pop(key, None)
        a = root/"a.txt"
        a.write_text("BASELINE-PRIVATE-CONTENT\n", encoding="utf-8")
        log = base/"diagnostics.jsonl"
        term = Terminal(binary, root, env, ["--no-mouse", "--debug", "--log-file", str(log)])
        try:
            term.wait("Monitoring")
            term.wait("0 files changed")
            assert b"\x1b[?1002h" not in term.data
            checks += 1
            mark = len(term.data)
            a.write_text("EDITED-PRIVATE-CONTENT\n", encoding="utf-8")
            term.wait("1 files changed", mark)
            term.wait("M a.txt", mark)
            checks += 1
            mark = term.send(b"\r")
            term.wait("-BASELINE-PRIVATE-CONTENT", mark)
            term.wait("+EDITED-PRIVATE-CONTENT", mark)
            assert not re.search(rb"\x1b\[(?:3[12]|38;[^m]+)m", bytes(term.data))
            checks += 1
            mark = term.send(b"?")
            term.wait("KEYBOARD", mark)
            term.send(b"\x1b")
            term.pump(0.2)
            checks += 1
            if os.geteuid() != 0:
                mark = len(term.data)
                a.chmod(0)
                try:
                    term.wait("Warning:", mark)
                    assert term.proc.poll() is None
                    checks += 1
                finally:
                    a.chmod(0o600)
                term.pump(0.3)
            else:
                print("PTY permission check SKIPPED: root bypasses mode-000")
            mark = term.send(b"p")
            term.wait("Paused", mark)
            paused_mark = len(term.data)
            (root/"new").mkdir()
            (root/"new"/"nested.txt").write_text("during pause", encoding="utf-8")
            a.write_text("AFTER-PAUSE-CONTENT\n", encoding="utf-8")
            term.pump(0.4)
            assert "2 files changed" not in term.text(paused_mark), "paused list advanced"
            mark = term.send(b"p")
            term.wait("Monitoring", mark)
            term.wait("2 files changed", mark)
            term.wait("+AFTER-PAUSE-CONTENT", mark)
            checks += 1
            mark = term.send(b"r")
            term.wait("Reset baseline", mark)
            mark = term.send(b"n")
            term.wait("Reset cancelled", mark)
            checks += 1
            term.send(b"r")
            mark = term.send(b"y")
            term.wait("0 files changed", mark)
            term.wait("baseline 2", mark)
            checks += 1
            mark = len(term.data)
            a.write_text("AFTER-RESET-CONTENT\n", encoding="utf-8")
            term.wait("1 files changed", mark)
            term.wait("-AFTER-PAUSE-CONTENT", mark)
            term.wait("+AFTER-RESET-CONTENT", mark)
            checks += 1
            # Filter no-match and cancellation are real key input, not model calls.
            mark = term.search(b"nothing-matches")
            term.wait("No changed paths match", mark)
            mark = term.send(b"\x1b")
            term.wait("M a.txt", mark)
            checks += 1
            # A large added text uses the core's cheap added-file diff path.
            mark = len(term.data)
            (root/"long.txt").write_text("".join(f"LONG-ROW-{i:04d}\n" for i in range(1500)), encoding="utf-8")
            for i in range(110):
                (root/f"z-{i:03d}.txt").write_text("added", encoding="utf-8")
            term.wait("112 files changed", mark)
            mark = term.search(b"long.txt")
            term.wait("+LONG-ROW-0000", mark)
            mark = term.send(b"\x1b[6~")
            term.wait("+LONG-ROW-002", mark)
            mark = term.send(b"G")
            term.wait("+LONG-ROW-1499", mark)
            checks += 1
            mark = term.resize(45, 12)
            term.wait("FolderWatch", mark)
            mark = term.resize(25, 6)
            term.wait("Resize to at least", mark)
            mark = term.resize(120, 36)
            term.wait("112 files changed", mark)
            checks += 1
            mark = term.send(b"e")
            term.wait("Diagnostics", mark)
            term.send(b"\x1b")
            term.finish(b"q", 0)
            checks += 1
        finally:
            term.close()
        assert not list(cache.glob("folderwatch-*")), "session cache leaked"
        logs = log.read_text(encoding="utf-8")
        records = [json.loads(line) for line in logs.splitlines()]
        assert records and any(e["level"] == "debug" for e in records)
        assert "PRIVATE-CONTENT" not in logs and "LONG-ROW" not in logs and "AFTER-PAUSE" not in logs
        assert log.stat().st_mode & 0o777 == 0o600
        checks += 1

        # Default mouse mode, friendly metadata, actual SGR mouse click, colors.
        env.pop("NO_COLOR", None)
        color_root = base/"color"
        color_root.mkdir()
        target = color_root/"a.txt"
        target.write_text("old\n", encoding="utf-8")
        term = Terminal(binary, color_root, env, ["--tui", "--max-diff-bytes=64B"])
        try:
            term.wait("Monitoring")
            assert b"\x1b[?1002h" in term.data
            target.write_text("new\n", encoding="utf-8")
            term.wait("1 files changed")
            mark = term.send(b"\x1b[<0;6;4M\x1b[<0;6;4m")
            term.wait("-old", mark)
            term.wait("+new", mark)
            assert b"\x1b[31m" in term.data and b"\x1b[32m" in term.data
            checks += 1
            mark = len(term.data)
            (color_root/"binary.dat").write_bytes(b"\x00\xff\x01")
            (color_root/"huge.txt").write_text("long "*100, encoding="utf-8")
            term.wait("3 files changed", mark)
            mark = term.search(b"binary")
            term.wait("Binary file changed", mark)
            mark = term.send(b"\x1b")
            term.pump(0.2)
            mark = term.search(b"huge")
            term.wait("diff skipped", mark)
            checks += 1
            term.finish(b"\x03", 130)
            checks += 1
        finally:
            term.close()
        assert not list(cache.glob("folderwatch-*"))
        checks += 1

        # Quit during a cancellable startup capture; sparse data is never copied
        # into the TUI, and the startup owner must join/clean up before return.
        startup_root = base/"startup"
        startup_root.mkdir()
        with (startup_root/"large-sparse").open("wb") as large:
            large.truncate(512 << 20)
        term = Terminal(binary, startup_root, env, ["--no-mouse"])
        try:
            term.wait("Scanning")
            term.finish(b"q", 0)
            assert not list(cache.glob("folderwatch-*"))
            checks += 1
        finally:
            term.close()

        # Fatal root loss must restore the terminal and return nonzero even paused.
        fatal_root = base/"fatal"
        fatal_root.mkdir()
        term = Terminal(binary, fatal_root, env, ["--no-mouse"])
        try:
            term.wait("Monitoring")
            mark = term.send(b"p")
            term.wait("Paused", mark)
            fatal_root.rmdir()
            term.finish(None, 1)
            assert "folderwatch:" in term.text()
            checks += 1
        finally:
            term.close()
        assert not list(cache.glob("folderwatch-*"))
        checks += 1

    print(f"R4 TUI PTY smoke: {checks} checks passed (not Terminal.app/iTerm2 manual QA)")


if __name__ == "__main__":
    main()
