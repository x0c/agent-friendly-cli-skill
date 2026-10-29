# AGENTS.md

This repository is the source and release repo of the agent-friendly-cli skill. Follow these conventions when working in it.

## Repository structure

- The repo root is the skill body (`SKILL.md` router + `references/` six references + `agents/openai.yaml` + `tests/` regression fixtures). The local development authority is `~/.config/agentsync/skills/agent-friendly-cli/` — land changes in the authority first, then sync to this repo; never edit only the repo copy.

## Hard constraints

- **English only** (user ruling, 2026-10-29): every file in this repo — skill body, references, tests, commit messages — must be English. `README.md` is the single exception and may stay bilingual. Never introduce Chinese anywhere else, including "temporary" notes.
- **Merge before push, never blind-push**: before every push, clone the remote and `diff -rq` it against the local authority — the remote may hold updates pushed from other machines (including remote-only fixes such as README install URLs). Absorb remote-only content into the local authority first, then sync-push; direct rsync overwrites are forbidden.
- **Commit identity, both fields, is x0c** (`GIT_COMMITTER_NAME`/`GIT_COMMITTER_EMAIL` + `--author`, x0c@users.noreply.github.com); real names and emails never enter commit history.
- **No internal-network content**: class names, paths, and configs of internal platforms must not enter this repo; use generic phrasing for examples (no company or platform names), and generic references ("a scheduling-platform CLI") with dates kept in incident narratives.

## Documentation map

- `README.md`: the storefront — the skill's use cases and installation for outsiders. Keep the clone URL in sync on repo renames or install changes; **never lose the Quick Install section**.
- `SKILL.md`: the router (scope, which-reference-when, contract evolution) — keep it lean.
- `references/design-principles.md`: the design standard (P0/P1/P2 + dual audience + read-before-write fingerprint recipe). `references/implementation-workflow.md`: the workflow. `references/pitfalls.md`: field incidents. `references/distribution.md`: distribution and lifecycle. `references/go-engineering.md`: Go engineering. `references/verification.md`: acceptance.
- `tests/`: the skill's own regression fixtures (a fresh-machine simulation brief plus the pre-registered acceptance criteria). Re-run them per their README after changing the skill.
