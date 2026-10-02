#!/usr/bin/env python3
"""Read-only verification that builds use the reviewed, reproducible native patch."""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import subprocess


def main() -> None:
    root = Path(__file__).resolve().parent.parent
    manifest = json.loads((root / "patches/fsnotify-v1.8.0.json").read_text())
    source = root / "vendor/github.com/fsnotify/fsnotify/backend_kqueue.go"
    patch = root / "patches/fsnotify-v1.8.0.patch"
    for path, expected in ((source, manifest["patched_sha256"]), (patch, manifest["patch_sha256"])):
        actual = hashlib.sha256(path.read_bytes()).hexdigest()
        if actual != expected:
            raise SystemExit(f"Unexpected vendor/patch change: {path.relative_to(root)}; review and regenerate per ADR-009")
    result = subprocess.run(
        ["go", "list", "-f", "{{.Dir}}", "github.com/fsnotify/fsnotify"],
        cwd=root, check=True, capture_output=True, text=True, timeout=30,
    )
    if Path(result.stdout.strip()).resolve() != source.parent.resolve():
        raise SystemExit("Build bypasses the reviewed vendor patch; remove -mod=mod/-mod=readonly overrides")
    subprocess.run(["git", "apply", "--reverse", "--check", str(patch)], cwd=root, check=True, timeout=10)
    print("Vendor guard: patched SHA-256, patch integrity/reversibility and active Go source path PASS")


if __name__ == "__main__":
    main()
