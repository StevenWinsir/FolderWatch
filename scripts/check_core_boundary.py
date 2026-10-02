#!/usr/bin/env python3
"""Enforce the transitive Core/UI boundary, not just direct import spelling."""
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
FORBIDDEN = ("github.com/charmbracelet/", "github.com/wailsapp/",
             "github.com/StevenWinsir/FolderWatch/internal/tui",
             "github.com/StevenWinsir/FolderWatch/internal/cli")


def main():
    deps = subprocess.check_output(["go", "list", "-mod=vendor", "-deps", "./internal/app"], cwd=ROOT, text=True).splitlines()
    forbidden = [dep for dep in deps if dep.startswith(FORBIDDEN)]
    if forbidden:
        raise SystemExit("Core has UI dependencies: " + ", ".join(forbidden))
    print("PASS: internal/app transitive dependency graph is UI-independent")


if __name__ == "__main__":
    main()
