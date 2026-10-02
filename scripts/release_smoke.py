#!/usr/bin/env python3
"""Verify candidate checksums/archives; on macOS install and run without Go on PATH."""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import queue
import re
import signal
import subprocess
import tarfile
import tempfile
import threading
import time

from release import sha256, version_value


def safe_name(name):
    path = PurePosixPath(name)
    return bool(name) and name == str(path) and not path.is_absolute() and ".." not in path.parts and "\\" not in name and name != "."


def verify(directory):
    directory = Path(directory).resolve(strict=True)
    checksums = {}
    for line in (directory / "SHA256SUMS").read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match[2] in checksums:
            raise ValueError("invalid or duplicate checksum entry")
        checksums[match[2]] = match[1]
    if not {"manifest.json", "folderwatch.rb"} <= checksums.keys():
        raise ValueError("missing manifest/formula checksums")
    for name, digest in checksums.items():
        path = directory / name
        if path.is_symlink() or not path.is_file() or sha256(path) != digest:
            raise ValueError("checksum mismatch: " + name)
    manifest = json.loads((directory / "manifest.json").read_text())
    if manifest.get("schema") != 1 or manifest.get("dependency_mode") != "vendor" or manifest.get("cgo_enabled") is not False:
        raise ValueError("invalid build provenance")
    version_value(manifest["version"])
    assets = manifest["assets"]
    if len(assets) != 2 or {asset["arch"] for asset in assets} != {"arm64", "amd64"}:
        raise ValueError("expected exactly two macOS architectures")
    expected = {"manifest.json", "folderwatch.rb"}
    for asset in assets:
        name = "folderwatch-" + manifest["version"] + "-darwin-" + asset["arch"] + ".tar.gz"
        if asset["os"] != "darwin" or asset["name"] != name or checksums.get(name) != asset["sha256"]:
            raise ValueError("asset identity/checksum mismatch")
        path = directory / name
        if path.stat().st_size != asset["bytes"]:
            raise ValueError("asset size mismatch")
        inspect_archive(path, asset["arch"])
        expected.add(name)
    if set(checksums) != expected:
        raise ValueError("unexpected/missing release checksum entries")
    return manifest


def inspect_archive(path, arch):
    with tarfile.open(path, "r:gz") as archive:
        names, total, found = set(), 0, False
        for member in archive:
            if not safe_name(member.name) or member.name in names or not member.isfile():
                raise ValueError("unsafe archive member: " + member.name)
            if member.size < 0 or member.size > 64 << 20 or member.mode not in (0o644, 0o755):
                raise ValueError("unsafe member size/mode")
            total += member.size
            if total > 128 << 20 or len(names) >= 512:
                raise ValueError("archive exceeds verification budget")
            names.add(member.name)
            if member.name == "folderwatch":
                if member.mode != 0o755:
                    raise ValueError("executable permission missing")
                stream = archive.extractfile(member)
                if stream is None:
                    raise ValueError("binary missing")
                with stream:
                    header = stream.read(8)
                # Little-endian 64-bit Mach-O: magic then cputype.
                expected_cpu = {"arm64": 0x0100000C, "amd64": 0x01000007}[arch]
                if len(header) != 8 or header[:4] != b"\xcf\xfa\xed\xfe" or int.from_bytes(header[4:8], "little") != expected_cpu:
                    raise ValueError("wrong Mach-O architecture")
                found = True
        if not found or not {"README.md", "CHANGELOG.md", "DISTRIBUTION.md", "licenses/Go-LICENSE"} <= names:
            raise ValueError("release metadata/licenses missing")
        if not any(name.startswith("licenses/github.com/fsnotify/fsnotify/") for name in names):
            raise ValueError("fsnotify license missing")


def install_and_run(directory, manifest):
    if platform.system() != "Darwin":
        raise ValueError("native install smoke requires macOS; use --verify-only elsewhere")
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64"}.get(platform.machine())
    if arch is None:
        raise ValueError("unsupported host architecture")
    asset = next(asset for asset in manifest["assets"] if asset["arch"] == arch)
    with tempfile.TemporaryDirectory(prefix="folderwatch-install-") as temp:
        base = Path(temp)
        home, root, cache, install = (base / name for name in ("home", "项目 with spaces", "cache", "bin"))
        for path in (home, root, cache, install):
            path.mkdir()
        binary = install / "folderwatch"
        # Extract the already-verified regular executable only, never tar.extractall.
        with tarfile.open(Path(directory) / asset["name"], "r:gz") as archive:
            source = archive.extractfile("folderwatch")
            if source is None:
                raise ValueError("no binary")
            with source:
                binary.write_bytes(source.read(64 << 20))
        binary.chmod(0o755)
        env = {"PATH": "/usr/bin:/bin", "HOME": str(home), "XDG_CONFIG_HOME": str(home),
               "TMPDIR": str(cache), "TMP": str(cache), "TEMP": str(cache), "LANG": "en_US.UTF-8"}
        version = subprocess.check_output([str(binary), "--version"], cwd=root, env=env, text=True, timeout=10).strip()
        expected = "folderwatch " + manifest["version"] + " (commit " + manifest["build_commit"] + ", built " + manifest["build_date"] + ")"
        if version != expected:
            raise ValueError("version metadata does not match manifest: " + version)
        (root / "a.txt").write_text("baseline\n")
        scan = json.loads(subprocess.check_output([str(binary), "--scan", "--json", str(root)], cwd=root, env=env, text=True, timeout=10))
        if {entry["path"] for entry in scan["entries"]} != {".", "a.txt"}:
            raise ValueError("installed scan returned wrong inventory")
        messages = queue.Queue(maxsize=128)
        with tempfile.TemporaryFile(mode="w+t") as errors:
            proc = subprocess.Popen([str(binary), "--watch", "--json", "--debounce=20ms", str(root)],
                                    cwd=root, env=env, text=True, stdout=subprocess.PIPE, stderr=errors)
            def reader():
                for line in proc.stdout:
                    try:
                        messages.put_nowait(json.loads(line))
                    except (ValueError, queue.Full):
                        return
                try:
                    messages.put_nowait(None)
                except queue.Full:
                    pass
            thread = threading.Thread(target=reader, daemon=True)
            thread.start()
            try:
                event = messages.get(timeout=10)
                if not isinstance(event, dict) or event.get("type") != "ready":
                    raise ValueError("installed watcher did not become ready")
                content = b"installed candidate edited\n"
                (root / "a.txt").write_bytes(content)
                deadline = time.monotonic() + 10
                while True:
                    event = messages.get(timeout=max(0.01, deadline-time.monotonic()))
                    if not isinstance(event, dict):
                        raise ValueError("installed watcher ended early")
                    updates = event.get("state", {}).get("changes", event.get("batch", {}).get("upserts", []))
                    if any(item["path"] == "a.txt" and item["kind"] == "modified" and item.get("after", {}).get("hash") == hashlib.sha256(content).hexdigest() for item in updates):
                        break
                    if time.monotonic() >= deadline:
                        raise ValueError("installed watcher missed edit")
                proc.send_signal(signal.SIGINT)
                if proc.wait(timeout=10) != 130:
                    raise ValueError("installed watcher did not cleanly interrupt")
                thread.join(timeout=3)
                if thread.is_alive() or list(cache.iterdir()):
                    raise ValueError("installed process leaked reader/cache")
                errors.seek(0)
                if errors.read():
                    raise ValueError("installed process wrote unexpected diagnostics")
            finally:
                if proc.poll() is None:
                    proc.kill()
                    proc.wait(timeout=5)
                thread.join(timeout=3)
                proc.stdout.close()
        print("PASS: native " + arch + " archive installation, exact version, Unicode scan, real watch/hash, Ctrl+C and cache cleanup; isolated HOME/PATH, not a clean-Mac human signoff")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--verify-only", action="store_true")
    args = parser.parse_args()
    manifest = verify(args.directory)
    print("PASS: both macOS archive checksums, architecture, safe contents and provenance")
    if not args.verify_only:
        install_and_run(args.directory, manifest)


if __name__ == "__main__":
    main()
