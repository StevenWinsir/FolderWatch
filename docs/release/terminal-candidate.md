# Terminal candidate build, installation and release checklist

## Distribution status

R5 provides **private evaluation candidates**, not a public v1 release. The app runs without Go, Python, Git or Homebrew installed. Developers need Go 1.23+ and Python 3 to build/test; the release pipeline uses the recorded Go toolchain and the repository's vendor patch. Gate A's human Terminal.app/iTerm2 and clean-Mac checks, plus the owner's project-license decision, remain separate requirements. Read [DISTRIBUTION.md](DISTRIBUTION.md) and [Gate A](../gates/Gate-A.md).

## Build from an exact checkout

```sh
go mod download
make lint scripts-test test race smoke
# A clean Git checkout is required. This never creates a Git tag or uploads a release.
make release VERSION=v0.1.0-rc.1
make release-smoke VERSION=v0.1.0-rc.1
ruby -c dist/v0.1.0-rc.1/folderwatch.rb
```

Outputs are `folderwatch-v0.1.0-rc.1-darwin-arm64.tar.gz`, `folderwatch-v0.1.0-rc.1-darwin-amd64.tar.gz`, `manifest.json`, `folderwatch.rb` and `SHA256SUMS`. Archives include the executable, README/CHANGELOG, distribution boundary and vendored/toolchain licenses. `--version` reports the exact version, full commit and commit date from the manifest. CGO is disabled. For byte-reproducibility compare two builds using the same checkout, Go toolchain and build host platform; tar entries are sorted with fixed ownership/mode/timestamps and gzip has no current filename/time metadata. Cross-host ad-hoc code-signing/toolchain changes are not promised byte-identical.

`python3 scripts/release.py --version v0.1.0-rc.1 --output dist/evaluation --allow-dirty` is only a local diagnostic escape hatch. Such manifests say `dirty: true` and binaries carry `-dirty` in their commit field; they are not approved release evidence. Existing output directories are never intentionally overwritten. Generated dist/artifacts and Python bytecode are ignored, not committed. Do not use `go install ...@version`, `-mod=mod` or a fresh unpatched `go mod vendor` as an equivalent build.

## Install a downloaded, approved candidate without a development toolchain

Obtain the complete candidate directory from the authorized GitHub Actions artifact or the owner's eventual approved release. Repository/artifact access is private; generated default GitHub release URLs are not proof those URLs already host assets. Check the expected manifest/checksum against a trusted source before extraction; hashes alone do not establish publisher identity.

On the target Mac, in the candidate directory:

```sh
shasum -a 256 -c SHA256SUMS
# Apple Silicon uses arm64; Intel uses amd64. Inspect uname -m before choosing.
uname -m
stage="$(mktemp -d)"
tar -xzf folderwatch-v0.1.0-rc.1-darwin-arm64.tar.gz -C "$stage"
"$stage/folderwatch" --version
"$stage/folderwatch" --scan --json "$HOME"
# Install deliberately; back up an existing binary before replacing it.
mkdir -p "$HOME/.local/bin"
install -m 755 "$stage/folderwatch" "$HOME/.local/bin/folderwatch"
"$HOME/.local/bin/folderwatch" "/path/to/项目 with spaces"
```

The last command needs an interactive terminal. Add `$HOME/.local/bin` to PATH using your shell's normal configuration, or use the absolute path. Intel users select the amd64 archive instead. Do not bypass macOS security prompts or remove quarantine blindly; these candidates have no claimed Developer ID signature or Apple notarization. Record the actual prompt and request an approved distribution/signing decision. Removing the installed binary uninstalls the application; user configuration is never deleted automatically.

The automated smoke extracts only the verified regular executable to a new temporary directory, sets HOME/XDG/TMP to new private locations and PATH to `/usr/bin:/bin`, checks exact version, Unicode scan, real modified-content hash, Ctrl+C=130 and empty caches. This proves no developer-toolchain runtime dependency in that environment, **not** fresh-Mac/Gatekeeper or human acceptance.

## Homebrew plan

The generated `folderwatch.rb` selects `on_arm` or `on_intel`, includes the real archive SHA-256 and installs only `folderwatch`; its test runs version and JSON scan. Formula syntax and both checksums are validated. The default URLs point to `https://github.com/StevenWinsir/FolderWatch/releases/download/<version>/...` and remain unusable until those exact assets are uploaded with appropriate repository access. No public tap is created by R5.

For a maintainer-controlled local evaluation, build with `--url-root file:///absolute/path/to/candidate-directory` (URI-encode spaces). The generator uses that literal root and actual checksums. Place the formula in an owner-managed private evaluation tap and test using the installed Homebrew version's supported local-tap workflow. Record Homebrew version, formula SHA, installed architecture and `brew test` output before declaring that integration accepted. A public tap must wait for project-license and distribution approval; do not commit tokens or embed private download credentials in the formula.

## CI / manual candidate workflow

Every PR runs the test/race matrix, cross-compilation, bounded fuzzing and native macOS candidate installation. A separate `Terminal release candidate` workflow accepts a validated version through workflow_dispatch, reruns lint/script tests/test/race/smokes, builds both archives, performs native install checks and uploads the complete candidate directory for 30 days. It has no repository write permission and cannot publish tags/releases or approve Gate A. The normal CI candidate is retained for 14 days; archive important approved evidence before retention expiry.

Before public distribution, record the owner's license decision; complete and sign Terminal.app/iTerm2 plus genuinely clean Apple Silicon and Intel acceptance where supported; review known issues and artifacts/checksums/provenance; decide Developer ID/notarization policy; then deliberately upload the exact approved candidate and publish a tested tap. R5 does not claim any of those external approvals have already occurred.
