#!/usr/bin/env python3
"""Generate dist/manifest.json: records sha256 hashes and build commit for the
six-platform release artifacts.

Called automatically by the root build.mk's build-all target after compilation
finishes; the manifest is committed alongside the binaries. The self-update
mechanism (autoupdate package) reads this manifest before pulling remote dist
artifacts, using it for both the version guard and hash verification.

Usage (from a subproject directory, invoked by the Makefile):
  python3 ../scripts/gen-dist-manifest.py --command mytool
"""

import argparse
import hashlib
import json
import subprocess
import sys
from pathlib import Path

# Keep in sync with TARGETS in build.mk
TARGETS = [
    ("darwin", "amd64"), ("darwin", "arm64"),
    ("linux", "amd64"), ("linux", "arm64"),
    ("windows", "amd64"), ("windows", "arm64"),
]


def git_output(repo_root: Path, *args: str) -> str:
    r = subprocess.run(["git", "-C", str(repo_root), *args],
                       capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)} failed: {r.stderr.strip()}")
    return r.stdout.strip()


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Generate dist/manifest.json")
    parser.add_argument("--command", required=True,
                        help="command name (e.g. mytool)")
    parser.add_argument("--dist-dir", default="dist",
                        help="artifact directory (default: ./dist)")
    parser.add_argument("--version", default=None,
                        help="git describe version; computed on the fly if omitted")
    parser.add_argument("--repo-root", default=None,
                        help="repository root; defaults to two levels above the "
                             "dist directory (multi-project monorepo layout); pass "
                             "explicitly for single-repo layouts")
    args = parser.parse_args()

    dist = Path(args.dist_dir)
    if not dist.is_dir():
        print(f"error: directory {dist} does not exist; run make build-all first",
              file=sys.stderr)
        return 1

    # Repo root = two levels above the dist directory (one level above the
    # subproject root); override with --repo-root for single-repo layouts
    project_root = dist.resolve().parent
    repo_root = Path(args.repo_root).resolve() if args.repo_root else project_root.parent
    head_full = git_output(repo_root, "rev-parse", "HEAD")
    head_short = git_output(repo_root, "rev-parse", "--short", "HEAD")
    version = args.version or git_output(
        repo_root, "describe", "--tags", "--always")

    artifacts = {}
    for os_name, arch in TARGETS:
        ext = ".exe" if os_name == "windows" else ""
        name = f"{args.command}-{os_name}-{arch}{ext}"
        path = dist / name
        if not path.is_file():
            print(f"error: missing artifact {path}", file=sys.stderr)
            return 1
        artifacts[name] = sha256_file(path)

    manifest = {
        "command": args.command,
        "version": version,
        "build_commit": head_short,
        "head": head_full,
        "artifacts": artifacts,
    }
    out = dist / "manifest.json"
    out.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
                   encoding="utf-8")
    print(f"generated {out} ({version} @ {head_short})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
