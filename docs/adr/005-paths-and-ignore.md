# ADR-005: Canonical paths, symlinks and Ignore

Status: Accepted (R1, 2026-10-01)

## Context
Initial scan and future runtime-create filtering must agree, including nested rules, excluded parents and symlinks.

## Decision
Resolve the explicitly selected root to a clean absolute real directory (including an explicitly selected root symlink). Internal keys are root-relative, slash-separated and case-preserving; root is `.`. Do not case-fold or normalize Unicode. Display escaped relative keys in Terminal. Reject keys outside root. Existing symlink ancestors are rejected for runtime paths; final symlinks are inventoried as links, never traversed. This is a local observation policy, not an atomic filesystem sandbox against malicious concurrent directory replacement.

Use one `ignore.Matcher.Match(path, isDir)` for scanner and future runtime filtering. Lowest-to-highest rule priority: built-in `.git/`; enabled root/nested `.gitignore` (deeper scopes override parents); root `.folderwatchignore`; explicit `ignore_file`; CLI/config ignore list. Last matching rule wins at each path component. An excluded ancestor cannot be resurrected by a child-only negation; unignore the directory first. `.git/` is ignored at any depth by default; `--include-git`/`include_git = true` disables that built-in rule. Other user rules can still exclude it. `respect_gitignore` is explicitly false by default; enabling it reads only root and nested .gitignore files, not global Git excludes or .git/info/exclude, and does not consult Git tracked-file status.

Support `*`, `?`, `[]`, `**`, root anchors, directory-only trailing slash, comments, negation, escaped leading `#`/`!`, escaped trailing spaces and CRLF. Bare patterns match basenames at any depth in their scope; slash patterns are relative to the rule file's directory. Braces are literals, not shell alternatives. Malformed globs are errors rather than silently broadening the monitored set. Root .folderwatchignore is always additive to an explicit ignore file. Root config/ignore files are bounded regular files (1 MiB); automatically discovered rule files cannot be symlinks. An explicitly selected external ignore file may resolve a symlink.

Git rule files are cached once per directory per matcher/session, including missing files and errors. A newly encountered runtime directory loads its own .gitignore; edits to an already-loaded ignore file require a new matcher/session. R2 must rebuild the matcher and reconcile if it adds live rule reloading; it must not invent a second filtering implementation. The cache is synchronized for concurrent Match calls.

Unreadable/disappearing descendants produce structured scan warnings and do not abort siblings. A root that cannot be read is fatal. Unreadable nested rule files cause the affected subtree to be skipped with warnings rather than scanned with incomplete policy. An invalid/unreadable root config or explicit ignore file fails startup with an actionable error.

## Alternatives
Absolute keys complicate display and portability. Following directory links risks loops and out-of-root traversal. Independently implemented runtime filters drift from initial scan. Full Git repository semantics would unexpectedly consult unrelated global settings.

## Consequences
No default writes to root. Warnings mean the inventory can be partial. Special files are metadata only and must not be opened by later content engines. Ignore edits do not hot-reload in R1. Config and ignore parsing are bounded, but total inventory size is proportional to the number of scanned entries; large-tree performance remains R5 work.

## Migration
The path key and rule ordering are stable R2 inputs. Broader Git semantics, link following, hot reload or case-folding require a separate ADR and regression tests.
