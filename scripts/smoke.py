#!/usr/bin/env python3
"""Exercise the built CLI on isolated fixtures; never scan or edit user data."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: python3 scripts/smoke.py path/to/folderwatch")
    binary = Path(sys.argv[1]).resolve(strict=True)
    checks = 0
    skipped = 0

    def check(condition: bool, message: str) -> None:
        nonlocal checks
        if not condition:
            raise AssertionError(message)
        checks += 1

    with tempfile.TemporaryDirectory(prefix="folderwatch-r1-") as temporary:
        base = Path(temporary)
        root = base / "项目 with spaces"
        home = base / "home"
        root.mkdir()
        home.mkdir()
        env = dict(os.environ, HOME=str(home), USERPROFILE=str(home),
                   XDG_CONFIG_HOME=str(home / ".config"))

        def write(key: str, text: str = "fixture") -> Path:
            path = root / key
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text, encoding="utf-8")
            return path

        def run(*args: str, code: int = 0) -> subprocess.CompletedProcess:
            result = subprocess.run([str(binary), *args], cwd=root, env=env,
                                    capture_output=True, text=True, timeout=15)
            check(result.returncode == code,
                  f"{args!r}: expected exit {code}, got {result.returncode}: {result.stderr}")
            return result

        def scan(*args: str) -> dict:
            return json.loads(run("--scan", "--json", *args).stdout)

        def paths(result: dict) -> set:
            return {entry["path"] for entry in result["entries"]}

        write(".folderwatchignore", "*.tmp\nignored/\n")
        write("nested/Unicode 文件.txt")
        write("ignored/hidden")
        write("drop.tmp")
        write(".git/HEAD", "fixture only; this is not a Git repository")
        write(".gitignore", "*.log\n")
        write("nested/.gitignore", "*.local\n")
        for name in ("root.log", "nested/drop.local", "extra.bin", "cli-only", "project-only", "user-only"):
            write(name)
        write("binary.data").write_bytes(bytes(range(256)))
        with (root / "huge.bin").open("wb") as large:
            large.truncate(5 * 1024 ** 3)  # Sparse 5 GiB fixture; metadata only.

        check("R1" in run("--help").stdout, "help must state scan-only scope")
        check("folderwatch" in run("--version").stdout, "missing version")
        result = scan()
        found = paths(result)
        check(result["root"] == str(root.resolve()), "noncanonical root")
        check("nested/Unicode 文件.txt" in found, "Unicode/space path missing")
        check("drop.tmp" not in found and "ignored" not in found, "default ignore file failed")
        check(".git" not in found, "built-in Git directory exclusion failed")
        check("root.log" in found and "nested/drop.local" in found, "Git ignores enabled silently")
        check(any(e["path"] == "huge.bin" and e["size"] == 5 * 1024 ** 3 for e in result["entries"]), "large file metadata missing")
        check("binary.data" in found, "binary extension/content excluded")
        respected = paths(scan("--respect-gitignore"))
        check("root.log" not in respected and "nested/drop.local" not in respected, "nested Git rules failed")
        check(".git/HEAD" in paths(scan("--include-git")), "explicit .git inclusion failed")
        check("huge.bin" in paths(scan("--max-diff-bytes", "1B")), "diff limit incorrectly filters scan")

        extra = base / "extra.ignore"
        extra.write_text("extra.bin\n", encoding="utf-8")
        found = paths(scan("--ignore-file", str(extra)))
        check("extra.bin" not in found and "drop.tmp" not in found, "explicit file did not compose with default rules")
        found = paths(scan(".", "--ignore", "cli-only", "--ignore", "extra.bin"))
        check("cli-only" not in found and "extra.bin" not in found, "interspersed repeatable CLI ignores failed")

        user_dir = (home / "Library/Application Support" if sys.platform == "darwin"
                    else home / ".config") / "FolderWatch"
        user_dir.mkdir(parents=True, exist_ok=True)
        (user_dir / "config.toml").write_text("ignore = ['user-only']\n", encoding="utf-8")
        write(".folderwatch.toml", "ignore = ['project-only']\nrespect_gitignore = true\n")
        found = paths(scan())
        check("user-only" in found and "project-only" not in found, "project must replace user ignore array")
        found = paths(scan("--ignore", "cli-only", "--respect-gitignore=false"))
        check("project-only" in found and "cli-only" not in found and "root.log" in found, "CLI precedence/explicit false failed")

        for args in (("--debounce", "bad"), ("--debounce", "0ms"), ("--max-diff-bytes", "0"),
                     ("--max-diff-bytes", "999999999999999999999TB"), ("missing",),
                     ("binary.data",), ("",), ("--ignore", "["), ("--ignore-file", "missing"),
                     ("--unknown",), (".", "another")):
            check(bool(run(*args, code=2).stderr.strip()), f"missing diagnostic for {args!r}")
        write(".folderwatch.toml", "unknown_setting = true\n")
        run("--json", code=2)
        run("--help")
        run("--version")
        write(".folderwatch.toml", "")

        if hasattr(os, "symlink") and os.name != "nt":
            outside = base / "outside"
            outside.mkdir()
            (outside / "secret").write_text("outside fixture", encoding="utf-8")
            os.symlink(outside, root / "external-link")
            os.symlink(root, root / "loop")
            os.symlink(root / "missing-target", root / "broken-link")
            result = scan()
            found = paths(result)
            check(all(x in found for x in ("external-link", "loop", "broken-link")), "symlink metadata missing")
            check(not any(x.startswith("external-link/") or x.startswith("loop/") for x in found), "symlink traversed")
            check(all(e["kind"] == "symlink" for e in result["entries"] if e["path"] in {"external-link", "loop", "broken-link"}), "symlink type lost")
        else:
            skipped += 1

        locked = write("locked/file").parent
        if os.name != "nt" and os.geteuid() != 0:
            locked.chmod(0)
            try:
                result = scan()
                check(any(w["path"] == "locked" for w in result["warnings"]), "permission warning missing")
                check("nested/Unicode 文件.txt" in paths(result), "permission error aborted siblings")
            finally:
                locked.chmod(0o700)
        else:
            skipped += 1

        before = sorted(str(p.relative_to(root)) for p in root.rglob("*"))
        run("--scan", "--log-file", "reserved.log", "--editor", "unused-editor --wait", "--debug")
        after = sorted(str(p.relative_to(root)) for p in root.rglob("*"))
        check(before == after, "scan or reserved options wrote into monitored root")

    print(f"CLI smoke: {checks} checks passed ({skipped} skipped)")


if __name__ == "__main__":
    main()
