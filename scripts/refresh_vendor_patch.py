#!/usr/bin/env python3
"""Regenerate the reviewed fsnotify patch against the pinned, hash-verified module cache."""
import difflib
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def main():
    manifest_path = ROOT / "patches/fsnotify-v1.8.0.json"
    manifest = json.loads(manifest_path.read_text())
    cache = Path(subprocess.check_output(["go", "env", "GOMODCACHE"], cwd=ROOT, text=True).strip())
    source = cache / "github.com/fsnotify/fsnotify@v1.8.0/backend_kqueue.go"
    original = source.read_bytes()
    if hashlib.sha256(original).hexdigest() != manifest["upstream_sha256"]:
        raise SystemExit("Pinned module-cache source checksum changed; refusing regeneration")
    path = "vendor/github.com/fsnotify/fsnotify/backend_kqueue.go"
    patched = (ROOT/path).read_bytes()
    patch = "".join(difflib.unified_diff(original.decode().splitlines(keepends=True), patched.decode().splitlines(keepends=True), fromfile="a/"+path, tofile="b/"+path)).encode()
    manifest["patched_sha256"] = hashlib.sha256(patched).hexdigest()
    manifest["patch_sha256"] = hashlib.sha256(patch).hexdigest()
    (ROOT / "patches/fsnotify-v1.8.0.patch").write_bytes(patch)
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps(manifest, indent=2))
    print("Review the generated diff, then run vendor_guard and all native/race regressions.")


if __name__ == "__main__":
    main()
