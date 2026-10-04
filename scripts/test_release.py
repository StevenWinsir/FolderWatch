#!/usr/bin/env python3
"""Offline regression tests for release boundaries and reproducible packaging."""
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import release
import release_smoke


class ReleaseTests(unittest.TestCase):
    def test_go_license_official_and_homebrew_layouts(self):
        with tempfile.TemporaryDirectory() as tmp:
            package = Path(tmp)
            goroot = package / "libexec"
            goroot.mkdir()
            notice = b"Copyright The Go Authors. Redistribution permitted."
            (package / "LICENSE").write_bytes(notice)
            self.assertEqual(release.go_license(goroot), notice)
            (goroot / "LICENSE").write_bytes(notice + b" official")
            self.assertEqual(release.go_license(goroot), notice + b" official")
            (goroot / "LICENSE").write_bytes(b"not Go license")
            with self.assertRaises(ValueError):
                release.go_license(goroot)

    def test_versions_reject_shell_and_path_input(self):
        for value in ("v0.1.0", "v1.2.3-rc.1", "v1.2.3-beta.2", "v1.0.0-alpha.1"):
            self.assertEqual(release.version_value(value), value)
        for value in ("dev", "1.0.0", "v01.0.0", "../v1.0.0", "v1.0.0;echo", "v1.0.0-rc.0", "v1.0.0\n"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                release.version_value(value)

    def test_tar_is_deterministic(self):
        with tempfile.TemporaryDirectory() as tmp:
            a, b = Path(tmp)/"a.gz", Path(tmp)/"b.gz"
            release.archive(a, {"z": (b"z", 0o644), "a": (b"a", 0o755)})
            release.archive(b, {"a": (b"a", 0o755), "z": (b"z", 0o644)})
            self.assertEqual(a.read_bytes(), b.read_bytes())
            with self.assertRaises(FileExistsError):
                release.archive(a, {})

    def test_safe_archive_paths(self):
        for name in ("../bad", "/bad", "dir/../../bad", "a\\b", "", ".", "a//b", "./a"):
            self.assertFalse(release_smoke.safe_name(name), name)
        self.assertTrue(release_smoke.safe_name("licenses/github.com/fsnotify/fsnotify/LICENSE"))

    def test_formula_checksums_and_literal_url(self):
        assets = [{"arch": arch, "name": arch + ".tar.gz", "sha256": "a"*64} for arch in release.ARCHES]
        formula = release.formula("v0.1.0-rc.1", assets, "file:///tmp/release")
        self.assertIn('version "0.1.0-rc.1"', formula)
        self.assertIn("on_intel do", formula)
        self.assertEqual(formula.count('sha256 "' + "a"*64 + '"'), 2)
        for url in ('https://example.com/#{system("bad")}', 'http://example.com', 'https://x/"', 'https://x/\n'):
            with self.assertRaises(ValueError):
                release.formula("v0.1.0", assets, url)

    def test_release_refuses_dirty_and_existing_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(ValueError):
                release.build("v0.1.0", Path(tmp))
            with patch.object(release, "run", return_value=" M source.go"):
                with self.assertRaises(ValueError):
                    release.build("v0.1.0", Path(tmp)/"new")
            self.assertFalse((Path(tmp)/"new").exists())

    def test_manifest_verification_and_tampering(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            assets = []
            for arch in release.ARCHES:
                name = "folderwatch-v0.1.0-rc.1-darwin-" + arch + ".tar.gz"
                cpu = 0x0100000C if arch == "arm64" else 0x01000007
                members = {"folderwatch": (b"\xcf\xfa\xed\xfe" + cpu.to_bytes(4, "little"), 0o755)}
                for doc in ("README.md", "CHANGELOG.md", "DISTRIBUTION.md", "licenses/Go-LICENSE", "licenses/github.com/fsnotify/fsnotify/LICENSE"):
                    members[doc] = (b"test fixture", 0o644)
                release.archive(root/name, members)
                assets.append({"arch": arch, "os": "darwin", "name": name, "sha256": release.sha256(root/name), "bytes": (root/name).stat().st_size})
            manifest = {"schema": 1, "version": "v0.1.0-rc.1", "dependency_mode": "vendor", "cgo_enabled": True, "watch_backend": "fsevents-with-polling-fallback", "assets": assets}
            (root/"manifest.json").write_text(json.dumps(manifest))
            (root/"folderwatch.rb").write_text("formula fixture")
            (root/"SHA256SUMS").write_text("".join(release.sha256(path) + "  " + path.name + "\n" for path in sorted(root.iterdir())))
            self.assertEqual(release_smoke.verify(root)["assets"], assets)
            (root/assets[0]["name"]).write_bytes(b"tampered")
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                release_smoke.verify(root)

    def test_archive_rejects_links_duplicates_and_wrong_cpu(self):
        import io
        import tarfile
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)/"bad.tar.gz"
            with tarfile.open(path, "w:gz") as out:
                member = tarfile.TarInfo("folderwatch")
                member.type = tarfile.SYMTYPE
                member.linkname = "/etc/passwd"
                out.addfile(member)
            with self.assertRaisesRegex(ValueError, "unsafe archive"):
                release_smoke.inspect_archive(path, "arm64")
            with tarfile.open(path, "w:gz") as out:
                member = tarfile.TarInfo("folderwatch")
                member.size, member.mode = 8, 0o755
                out.addfile(member, io.BytesIO(b"notMachO"))
            with self.assertRaisesRegex(ValueError, "architecture"):
                release_smoke.inspect_archive(path, "arm64")


if __name__ == "__main__":
    unittest.main()
