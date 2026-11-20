# AGENTS.md

This repository is the source and release repo of the agent-friendly-cli skill. Follow these conventions when working in it.

## Repository structure

- The repo root is the skill body (`SKILL.md` router + `references/` seven references + `agents/openai.yaml` + `tests/` regression fixtures). The local development authority is `~/.config/agentsync/skills/agent-friendly-cli/` — land changes in the authority first, then sync to this repo; never edit only the repo copy.

## Hard constraints

- **English only** (user ruling, 2026-10-29): every file in this repo — skill body, references, tests, commit messages — must be English. `README.md` is the single exception and may stay bilingual. Never introduce Chinese anywhere else, including "temporary" notes.
- **Merge before push, never blind-push**: before every push, clone the remote and `diff -rq` it against the local authority — the remote may hold updates pushed from other machines (including remote-only fixes such as README install URLs). Absorb remote-only content into the local authority first, then sync-push; direct rsync overwrites are forbidden.
- **Commit identity, both fields, is x0c** (`GIT_COMMITTER_NAME`/`GIT_COMMITTER_EMAIL` + `--author`, x0c@users.noreply.github.com); real names and emails never enter commit history.
- **No internal-network content**: class names, paths, and configs of internal platforms must not enter this repo; use generic phrasing for examples (no company or platform names), and generic references ("a scheduling-platform CLI") with dates kept in incident narratives.

## Documentation map

- `README.md`: the storefront — the skill's use cases and installation for outsiders. Keep the clone URL in sync on repo renames or install changes; **never lose the Quick Install section**.
- `SKILL.md`: the router (scope, which-reference-when, contract evolution) — keep it lean.
- `references/design-principles.md`: the design standard (P0/P1/P2 + dual audience + read-before-write fingerprint recipe). `references/implementation-workflow.md`: the workflow. `references/pitfalls.md`: field incidents. `references/distribution.md`: distribution and lifecycle. `references/go-engineering.md`: Go engineering. `references/verification.md`: acceptance. `references/maintaining-this-skill.md`: maintaining this skill itself — triggers, the five material sources, enter/leave criteria, the landing map, the audit-backfeed loop.
- `tests/`: the skill's own regression fixtures (a fresh-machine design brief, a maintenance-triage brief, and the pre-registered acceptance criteria). Re-run them per their README after changing the skill.

## Backlog (registered per maintaining-this-skill.md; the owner decides scheduling)

Batch 2 — next distillation round:

- Search-input normalization (separator equivalence) with zero-hit auto-retry using normalized variants — the one real-CLI capability none of the three 2026-11-20 blind designs reproduced.
- Post-processing pipelines sink into CLI flags (distilled views); stable semantic field names for high-traffic platform fields.
- verification.md audit section: fix-boundary before/after comparison; failed-session × companion-skill-read-rate correlation; multi-runtime call/result pairing quick reference + extractor sanity checks ("too-clean aggregate results suspect the extractor first").
- Consumer-side guidance in the companion-SKILL.md template (defect vs misuse, workarounds, where to report; parse defensively — check `ok`/`data` before field access).
- Acceptance-tooling precheck-failure semantics: FAIL or explicit SKIP, never silent PASS; explicit downgrade flag only.
- Baseline passing under multi-agent dispatch: three-state check (✅/❌/N/A) per clause — silent drops become explicit exemptions; baseline-lag warning in the review preamble.

Batch 3 — small items:

- exec-class shell-escaping dichotomy (per-word quoting vs whole-line passthrough, with the judgment rule stated).
- Text-channel file transfer: high-entropy markers vs PTY echo wrap-around, empty-content boundary, chunk-size tuning.
- Identity-mapping drift: the platform's live binding field is authoritative before writes; drift is a hard error.
- Diagnostic probes with unset targets fall back to a known baseline or report "target unset" — never "unreachable".
- Opaque JSON payload fields: pass through as text, never decode→re-encode; pin the shape with a save→reread→compare round trip.
- SKILL.md: one line declaring the relationship with machine-local CLI rules documents (three parallel authorities otherwise).

Adjacent (other skills / consuming repos; owner decision):

- swarm-manager: hard check "harvest every completed subagent result before writing shared context or dispatching the next wave" (2026-11-20 live incident: a probe result sat unconsumed for six minutes across long thinking blocks and falsified facts entered the shared context); session-isolated swarm working paths to prevent cross-session overwrites.
- The Kubernetes-platform CLI in the consuming monorepo carries its own 2026-11-19 friction-audit fix backlog (subcommand aliases, the repeatedly-invented list command, angle brackets on real values, availability sets in errors) — pairs with the skill entries landed 2026-11-20.
- Decision item: agent-behavior golden-regression methodology (supervision-agent testing) — extend verification.md or a separate skill.
