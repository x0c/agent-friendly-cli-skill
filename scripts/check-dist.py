#!/usr/bin/env python3
"""Pre-push check: for every subproject with committed dist artifacts, its
manifest must be consistent with the current source tree.

Guards against the incident pattern where source was pushed without rebuilding
dist — the branch sits in a stale state with newer code but older artifacts,
and self-update silently realigns every machine back to the old artifacts.

Check rules (for each subproject whose dist/ is tracked by git):
  1. dist/manifest.json exists and parses (artifacts predating the manifest
     mechanism must be rebuilt).
  2. manifest.head is a valid commit in the repository.
  3. Between manifest.head and HEAD there are no source changes outside the
     subproject's own dist/ directory (including repo-wide shared paths such
     as autoupdate/, build.mk, cli-common/, scripts/).
  4. The artifact set is exactly the six platforms, and working-tree file
     hashes match the manifest (this also catches uncommitted artifact edits).

Usage:
  python3 scripts/check-dist.py            # run from the repository root
  python3 scripts/check-dist.py --project mytool-cli   # check a single project
"""

import argparse
import hashlib
import json
import subprocess
import sys
from pathlib import Path

EXPECTED_TARGETS = [
    ("darwin", "amd64"), ("darwin", "arm64"),
    ("linux", "amd64"), ("linux", "arm64"),
    ("windows", "amd64"), ("windows", "arm64"),
]
# Shared paths (relative to the repo root) that affect all artifact builds;
# any change here requires every project to rebuild
COMMON_PATHS = ["autoupdate", "build.mk", "cli-common", "scripts"]


def git_output(repo_root: Path, *args: str) -> str:
    r = subprocess.run(["git", "-C", str(repo_root), *args],
                       capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)} failed: {r.stderr.strip()}")
    return r.stdout


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def dist_projects(repo_root: Path) -> list[str]:
    """Names of subproject directories whose dist artifacts are git-tracked."""
    out = git_output(repo_root, "ls-files", "--", "*/dist/*")
    projects = {line.split("/")[0] for line in out.splitlines() if line.strip()}
    return sorted(projects)


def check_project(repo_root: Path, project: str) -> list[str]:
    problems = []
    dist = repo_root / project / "dist"
    manifest_path = dist / "manifest.json"

    if not manifest_path.is_file():
        return [f"{project}: missing dist/manifest.json (artifacts predate the "
                f"manifest mechanism); run make build-all to rebuild"]

    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as e:
        return [f"{project}: failed to parse manifest.json: {e}"]

    head = manifest.get("head", "")
    if not head:
        return [f"{project}: manifest.json is missing the head field"]

    # head must be a valid commit in the repository
    r = subprocess.run(["git", "-C", str(repo_root), "cat-file", "-e",
                        f"{head}^{{commit}}"], capture_output=True)
    if r.returncode != 0:
        return [f"{project}: manifest.head {head} is not in the repository "
                f"(force push / history rewrite?); rebuild dist"]

    # Source changes from head → HEAD (excluding the project's own dist/)
    pathspecs = [project, f":(exclude){project}/dist", *COMMON_PATHS]
    diff = git_output(repo_root, "diff", "--name-only", f"{head}..HEAD", "--",
                      *pathspecs)
    changed = [l for l in diff.splitlines() if l.strip()]
    if changed:
        problems.append(
            f"{project}: source changed since the dist build "
            f"({manifest.get('build_commit', head[:7])}); run make build-all "
            f"before pushing: {', '.join(changed[:5])}"
            + (" ..." if len(changed) > 5 else ""))

    # Artifact set and hashes
    command = manifest.get("command", "")
    expected = {
        f"{command}-{os_name}-{arch}{'.exe' if os_name == 'windows' else ''}"
        for os_name, arch in EXPECTED_TARGETS
    } if command else None
    artifacts = manifest.get("artifacts", {})
    if expected is not None and set(artifacts) != expected:
        problems.append(f"{project}: manifest artifact set does not match the "
                        f"six platforms ({sorted(set(artifacts) ^ expected)})")
    for name, want_sha in sorted(artifacts.items()):
        path = dist / name
        if not path.is_file():
            problems.append(f"{project}: missing artifact {name}")
            continue
        got = sha256_file(path)
        if got != want_sha:
            problems.append(f"{project}: {name} hash does not match the manifest "
                            f"(dist edited without regenerating the manifest?)")
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Verify dist/source consistency before pushing")
    parser.add_argument("--project", default=None,
                        help="check only the given subproject (e.g. mytool-cli)")
    parser.add_argument("--repo-root", default=None,
                        help="repository root; defaults to the location of this "
                             "script (<repo-root>/scripts/); pass explicitly for "
                             "other layouts")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve() if args.repo_root else Path(__file__).resolve().parent.parent
    projects = [args.project] if args.project else dist_projects(repo_root)
    if args.project and not (repo_root / args.project).is_dir():
        print(f"error: subproject does not exist: {args.project}", file=sys.stderr)
        return 1

    all_problems = []
    for project in projects:
        all_problems.extend(check_project(repo_root, project))

    if all_problems:
        print("dist consistency check failed:", file=sys.stderr)
        for p in all_problems:
            print(f"  - {p}", file=sys.stderr)
        print("fix: run make build-all in each affected subproject and commit dist/",
              file=sys.stderr)
        return 1
    print(f"dist consistency check passed ({len(projects)} project(s))")
    return 0


if __name__ == "__main__":
    sys.exit(main())
