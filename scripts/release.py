#!/usr/bin/env python3
"""Build deterministic, vendor-preserving private Terminal release candidates."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parents[1]
ARCHES = ("arm64", "amd64")


def version_value(value):
    number = r"(?:0|[1-9][0-9]*)"
    if not re.fullmatch(r"v" + number + r"\." + number + r"\." + number + r"(?:-(?:rc|beta|alpha)\.[1-9][0-9]*)?", value):
        raise ValueError("version must be vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N (alpha/beta also supported)")
    return value


def sha256(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def archive(path, members):
    """members maps safe relative paths to (bytes, unix mode)."""
    with Path(path).open("xb") as raw:
        with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as compressed:
            with tarfile.open(mode="w", fileobj=compressed, format=tarfile.USTAR_FORMAT) as output:
                for name in sorted(members):
                    if name.startswith("/") or ".." in Path(name).parts or "\\" in name:
                        raise ValueError("unsafe archive member")
                    data, mode = members[name]
                    info = tarfile.TarInfo(name)
                    info.size, info.mode, info.mtime = len(data), mode, 0
                    info.uid, info.gid, info.uname, info.gname = 0, 0, "", ""
                    output.addfile(info, io.BytesIO(data))


def formula(version, assets, url_root):
    parsed = urlparse(url_root)
    if parsed.scheme not in ("https", "file") or (parsed.scheme == "https" and not parsed.netloc) or any(c in url_root for c in ('"', "'", "\\", "\n", "\r", "#")):
        raise ValueError("URL root must be a literal HTTPS or file URL without Ruby interpolation")
    by_arch = {asset["arch"]: asset for asset in assets}
    lines = ["# Private evaluation candidate; project distribution license is not yet selected.",
             "class Folderwatch < Formula", '  desc "Local folder changes relative to a session baseline"',
             '  homepage "https://github.com/StevenWinsir/FolderWatch"',
             '  version "' + version[1:] + '"', "  on_macos do"]
    for arch, block in (("arm64", "on_arm"), ("amd64", "on_intel")):
        asset = by_arch[arch]
        lines += ["    " + block + " do", '      url "' + url_root.rstrip("/") + "/" + asset["name"] + '"',
                  '      sha256 "' + asset["sha256"] + '"', "    end"]
    lines += ["  end", "", "  def install", '    odie "This candidate supports macOS only" unless OS.mac?',
              '    bin.install "folderwatch"', "  end", "", "  test do", '    system bin/"folderwatch", "--version"',
              '    system bin/"folderwatch", "--scan", "--json", testpath', "  end", "end", ""]
    return "\n".join(lines)


def run(args, env=None):
    return subprocess.check_output(args, cwd=ROOT, env=env, text=True).strip()


def go_license(goroot):
    # Official distributions keep LICENSE in GOROOT. Homebrew relocates it to
    # the package root immediately above libexec; never omit the notice.
    goroot = Path(goroot).resolve()
    candidates = [goroot / "LICENSE"]
    if goroot.name == "libexec":
        candidates.append(goroot.parent / "LICENSE")
    for path in candidates:
        if path.is_file():
            data = path.read_bytes()
            if b"Go Authors" not in data or b"Redistribution" not in data:
                raise ValueError("unrecognized Go toolchain LICENSE: " + str(path))
            return data
    raise ValueError("Go toolchain LICENSE not found; cannot build a distributable candidate")


def build(version, output, allow_dirty=False, url_root=None):
    version_value(version)
    output = Path(output).resolve()
    if output.exists():
        raise ValueError("output already exists; refusing to overwrite release evidence")
    dirty = bool(run(["git", "status", "--porcelain", "--untracked-files=normal"]))
    if dirty and not allow_dirty:
        raise ValueError("release requires a clean checkout; --allow-dirty is local evaluation only")
    commit = run(["git", "rev-parse", "HEAD"])
    build_date = run(["git", "show", "-s", "--format=%cI", "HEAD"])
    build_commit = commit + ("-dirty" if dirty else "")
    subprocess.run(["python3", "scripts/vendor_guard.py"], cwd=ROOT, check=True)
    toolchain = run(["go", "version"])
    goroot = Path(run(["go", "env", "GOROOT"]))
    members = {"README.md": ((ROOT / "README.md").read_bytes(), 0o644),
               "CHANGELOG.md": ((ROOT / "CHANGELOG.md").read_bytes(), 0o644),
               "DISTRIBUTION.md": ((ROOT / "docs/release/DISTRIBUTION.md").read_bytes(), 0o644),
               "licenses/Go-LICENSE": (go_license(goroot), 0o644)}
    for path in sorted((ROOT / "vendor").rglob("*")):
        if path.is_file() and path.name.lower().startswith(("license", "copying", "notice", "patents")):
            members["licenses/" + str(path.relative_to(ROOT / "vendor"))] = (path.read_bytes(), 0o644)
    output.parent.mkdir(parents=True, exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix=".folderwatch-release-", dir=output.parent))
    try:
        assets = []
        for arch in ARCHES:
            binary = stage / ("folderwatch-" + arch)
            env = dict(os.environ, GOOS="darwin", GOARCH=arch, CGO_ENABLED="0", GOWORK="off", GOFLAGS="-mod=vendor", GOTOOLCHAIN="local")
            ldflags = "-s -w -X main.version=" + version + " -X main.commit=" + build_commit + " -X main.buildDate=" + build_date
            subprocess.run(["go", "build", "-mod=vendor", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", str(binary), "./cmd/folderwatch"], cwd=ROOT, env=env, check=True)
            name = "folderwatch-" + version + "-darwin-" + arch + ".tar.gz"
            archive(stage / name, dict(members, folderwatch=(binary.read_bytes(), 0o755)))
            binary.unlink()
            assets.append({"name": name, "os": "darwin", "arch": arch, "sha256": sha256(stage / name), "bytes": (stage / name).stat().st_size})
        manifest = {"schema": 1, "version": version, "commit": commit, "build_commit": build_commit,
                    "build_date": build_date, "dirty": dirty, "toolchain": toolchain,
                    "dependency_mode": "vendor", "cgo_enabled": False,
                    "distribution": "private-evaluation; Gate A and project-license approval required before public distribution",
                    "assets": assets}
        (stage / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        root = url_root or "https://github.com/StevenWinsir/FolderWatch/releases/download/" + version
        (stage / "folderwatch.rb").write_text(formula(version, assets, root))
        sums = [sha256(path) + "  " + path.name for path in sorted(stage.iterdir())]
        (stage / "SHA256SUMS").write_text("\n".join(sums) + "\n")
        if output.exists():
            raise ValueError("output appeared while building; refusing to overwrite")
        stage.rename(output)
    finally:
        if stage.exists():
            shutil.rmtree(stage)
    print(json.dumps(manifest, indent=2))
    print("Release candidate: " + str(output))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--allow-dirty", action="store_true")
    parser.add_argument("--url-root", help="asset URL root; use file:///... for a local Homebrew evaluation")
    args = parser.parse_args()
    version_value(args.version)
    build(args.version, args.output or ROOT / "dist" / args.version, args.allow_dirty, args.url_root)


if __name__ == "__main__":
    main()
