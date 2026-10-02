# Terminal candidate distribution boundary

FolderWatch has not selected a project-wide public distribution license. This file does not grant a new license or change the repository owner's rights. R5 packages are **private evaluation candidates for authorized repository collaborators**, not a declaration of a public v1 release.

Before public distribution, the owner must record a project-license decision, complete the Terminal.app / iTerm2 and clean-Mac acceptance in `docs/gates/Gate-A.md`, and approve the candidate. No Developer ID signing or Apple notarization is claimed. Go's automatic/ad-hoc executable signing is not Developer ID signing.

Each archive contains the unmodified license/notice files present in the vendored dependencies and the building Go toolchain's LICENSE under `licenses/`. fsnotify v1.8.0 includes the reviewed local kqueue patch; its original license is retained. Do not rebuild with `go install ...@version` or `-mod=mod`, because those routes omit the vendored patch.

The generated Homebrew formula has real archive checksums. Its default GitHub release URLs become usable only after those exact assets are uploaded to that version. The private repository still requires access; no public tap, release upload, or clean-Mac installation is implied by generating a formula.
