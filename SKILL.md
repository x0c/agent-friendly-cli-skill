---
name: agent-friendly-cli
description: Design, retrofit, or review command-line tools (CLIs) and their companion skills, especially CLIs that AI agents must call reliably while staying usable for humans. Covers command-surface design, non-interactive mode, JSON output contracts, layered exit codes, dry-run, idempotency, auth security, CLI/script/skill/agent layering, transactional orchestration, risk authorization, terminal-state verification, and acceptance.
---

# Agent-Friendly CLI Development

## Scope

This skill governs **cross-language CLI contract design and acceptance**: how to slice commands, shape output, rank exit codes, handle non-interactive use, avoid pitfalls, and run acceptance. Language-level implementation details (e.g. Go's cobra/flag, Python's argparse) belong to language-specific skills; this skill does not repeat them.

One-line goal: make the CLI **low-token, low-ambiguity, low-risk, auditable, reproducible, and reversible** for agents — and **readable and interactive by default** for humans. One CLI, two audiences — not two tools.

## Which reference to read when

- Design the command surface / output contract / exit codes, or review whether a CLI is agent-friendly → read [references/design-principles.md](references/design-principles.md) (P0/P1/P2 tiers + dual-audience rules). When reviewing, check every item; each gap is a finding to report.
- Implement, and want to dodge real-world incidents → read [references/pitfalls.md](references/pitfalls.md) (dry-run must be truly read-only, idempotency signals, secret safety, test isolation, and other hard lessons).
- Finish building, ready for acceptance → read [references/verification.md](references/verification.md) (evidence-pasting acceptance checklist + real-agent evaluation methodology).

## Workflow

### Design

Clear the **seven P0 hard requirements** first — without them agents cannot use the tool at all. Details in design-principles.md:

| P0 | In one line |
|---|---|
| Non-interactive mode | Auto-disable interaction when non-TTY is detected (`isatty()`, stdin/stdout tested separately); never require the caller to pass a flag |
| Structured output | `--json` with a uniform envelope `{ok, data, error, meta}`; failures also go through stdout JSON + non-zero exit (human/pipeline-first tools may ship bare-data JSON instead — pick one, never mix; see reference) |
| Layered exit codes | 0 success / 1 generic failure / 2 usage error / 3 not found / 4 auth / 5 conflict / 6 timeout (map to this tool's real failure modes — see reference) |
| Dry-run | Side-effecting commands support rehearsal with the same output shape as real execution; truly read-only, costs nothing |
| Verification commands | `status`/`verify`/`doctor` so callers can double-check beyond the exit code |
| Input validation | Hard-block path traversal and command injection |
| Self-serve installation | When the companion skill cannot find the CLI, give a trusted download source, install into the user directory automatically, then verify version and capabilities |

Then add P1 as needed (`describe` self-description, structured errors with `hint`/`next_commands`, size control, write-ahead log, auto-generated SKILL.md, composability). Prefer **declarative commands** (`ensure`/`apply`) over imperative ones (`create`/`delete`); they are idempotent by nature.

**Dual audience** throughout: human-readable by default (tables and color allowed); strip everything down to the machine contract under `--json` or non-TTY. Every interactive wizard needs a non-interactive equivalent (`--yes` + full flags). Give dual fields for the same fact (stable English enum `status` for programs, localized `status_tag` for humans).

### CLI availability and self-serve installation

The companion skill's first step must probe the CLI's absolute path and `version`/`capabilities`. When no usable binary is found, do not reply "CLI not installed" or push the installation back to the user; a user request for that capability authorizes the agent to finish a **privilege-free, user-directory-only** local install and continue the original task — unless the user explicitly said not to install.

- **Machine-executable source**: state the trusted official repo or per-platform artifact download URL in the skill, the CLI-generated skill template, or `describe` output. Never give only a product homepage, make the agent search for a download link, or rely on an unstable package-manager name.
- **Deterministic install path**: ship a non-interactive install script or equivalent command that picks a prebuilt artifact by OS/CPU, installs into a user-writable directory, uses no `sudo`, and touches no system directories or shell configuration. When an artifact already exists at a different version, report a conflict by default; overwrite only with explicit `--force`/`--replace`.
- **Private distribution**: when the trusted source is private, the installer reads its token from an environment variable or the OS credential store (never a prompt, never printed) and keeps a second trusted path — a source build of the same pinned tag over the same private Git host — for machines without the token. Both paths verify what they installed.
- **Pin and verify**: after installing, record the absolute binary path, download source, and immutable version/commit; immediately run `version --json`, `capabilities --json`, or `doctor --json` to verify, then use only that absolute path for the rest of the session. Verify checksums or signatures whenever provided.
- **Explicit failure boundary**: only report a concrete blocker when the download source is unreachable, no artifact exists for the platform, verification fails, or a required tool (git/downloader) is missing. Never use a random URL from search results, a low-level API, or an ad-hoc `curl` to bypass the CLI. A CLI with no trusted download source and no executable install path is not a complete agent-usable deliverable.
- **Background self-update (when distributed from the same source)**: silent, same trusted source, no `--json` stdout pollution, environment-variable opt-out, separate process. Full conventions: see design-principles.md P0 §7.
- **Companion-skill reverse sync (when a companion skill exists)**: the CLI silently runs the skill update command in the background instead of reimplementing per-agent directory sync. Full conventions (eligibility, exemption, mutual exclusion, throttle, kill switch): see design-principles.md P0 §7.

### Ask-once login credentials

Once a CLI persists account credentials locally (0600) and auto re-logs in on session expiry, the companion guide must make the agent behave as "ask the user **once**, then never again". With the capability present but that one sentence missing from the guide, every new session degrades into asking for the password again (real incident: a registry CLI's guide only said "provide the password via environment variable", and another agent pushed setting the variable back to the user).

- CLI side: the login command accepts the password via environment variable (never blocks on interaction when non-TTY); credentials/profiles persist account and password together; expired sessions re-login automatically and refresh what is on disk.
- Error side: NEED_LOGIN-class error hints must spell out "ask the user once for account + password → run login with env vars → persisted, auto re-login after that", with copy-pasteable env-var commands in next_commands. A bare "please log in again" says nothing.
- Skill side: the login step states "password via environment variable, persisted locally with auto re-login — ask once; do not make the user set env vars themselves or ask repeatedly".
- Upgrade compatibility: old persisted credentials without a password report NEED_LOGIN the same way, with a hint that one fresh login upgrades them to the new format; credential file paths and field names are public contracts, and any format change ships with read/write compatibility tests for the old format (compat_test).

### Implement

Walk pitfalls.md and dodge each item. CLIs that reuse another tool's login or spend generation quota have dedicated entries there (shared login refresh, spend ledger, no auto-retry after uncertain paid calls, advisory parameters, binary artifacts). Priorities: the dry-run branch must intercept **every** side effect; write commands must converge to `ok` on the second run; secrets come only from environment variables and never enter logs/output/git, with credentials bound to the login-time service origin; upstream business success criteria stay decoupled from HTTP status codes, and write success is judged by re-reading terminal state; re-login happens once and only on session-expiry signals, and write requests with uncertain outcomes are never auto-replayed; tests redirect to temp directories and never touch real user resources.

### Companion-skill layering and transaction skeleton

A companion skill is not a CLI command list — it compiles the user's natural-language goal into an auditable, authorizable, recoverable, verifiable transaction. Write down the business terminal-state invariant first, e.g. "all deploy targets succeeded on the same version" or "platform state strictly matches code". Never define "some command exited 0" as done.

| Layer | Owns | Must not own |
|---|---|---|
| CLI | auth, target identity checks, version resolution, business hard gates, planning/diffing, idempotency, execution, status queries, structured errors | relying on the agent to remember critical safety rules |
| Helper scripts | cross-tool adaptation, legacy CLI compatibility, format conversion, temporary deterministic glue | long-term ownership of high-risk domain rules (pagination, snapshot consistency, deletion order) |
| Skill | intent routing, context lookup, flow orchestration, risk authorization, exception branches, result presentation | reimplementing computations the CLI already does deterministically, or calling low-level APIs to bypass the CLI |
| Agent | genuine semantic ambiguity and user decisions | parsing unstable prose, hand-computing diffs, guessing targets or defaults |

Follow this transaction skeleton by default; trim steps by task risk, never invert the safety order (non-droppable core: plan → authorize → verify terminal state):

```text
capability probe / login check → evidence-based target resolution → immutable plan
→ dry-run (offline, read-only) → preflight (remote, read-only) → authorization as needed
→ apply → status/reconcile → verify terminal state
```

- **Ask minimally**: resolve directly whatever the workspace, git remote, existing config, login state, or platform read-only APIs can answer. Ask only on genuinely ambiguous targets, missing business-required information, new-resource creation, or high-risk writes.
- **Authorize by risk**: read-only probes and explicitly authorized low-risk actions run automatically; high-risk actions (production, deletion, overwrite, migration) get exactly one confirmation after the plan and preflight. When the user cancels or takes over manually, stop waiting, polling, retrying, and resubmitting immediately.
- **Done means converged**: distinguish `planned`, `submitted`, `waiting_approval`, `running`, `succeeded`, `failed`, `unknown`; after a write succeeds, prove the terminal state satisfies the invariant with `status`, `verify`, or `reconcile`.
- **Retry safely**: retry only clearly failed, retryable targets; never resubmit succeeded, approval-pending, running, or unknown ones. Batch operations keep one plan and one idempotency key; a second run must converge to `unchanged` or return the existing operation.
- **Never bypass to patch gaps**: when the CLI falls short, extend the CLI or stop and report — never shell out to `curl`, read credentials directly, call low-level APIs, or hand-copy business rules to dodge its auth, audit, and safety gates.
- **Reuse before extending**: one agent mistake does not automatically mean the CLI lacks a command. First check whether existing flags, structured fields, or `--out` can close the loop; when only skill routing or result reading is wrong, fix the skill. When an existing rule keeps getting ignored, move the precondition ahead of the critical action and delete the later duplicate instead of stacking synonymous reminders. Add a command or flag only for stable, repeated flows the current contract cannot complete deterministically — and only when the complexity removed exceeds the complexity added.
- **Read sink-down signals**: stable, repeated, mechanical, cross-skill, or high-risk-consistency logic belongs in the CLI as a first-class command. When skills/scripts start owning pagination, diffing, loose matching, snapshot lifecycles, deletion order, or complex state machines long-term, the CLI is usually missing `plan/apply/verify`, `sync`, or `batch` composites; scripts may bridge the gap but must not become a second business core.
- **Sink batch reads too**: when the agent repeats the same discovery/diagnostic/filter command over one target set, the CLI should offer a one-call `batch`/`--all-matches`/aggregate command with bounded internal concurrency, per-target results, an overall summary, and partial/failures. Never treat N×M serial tool calls as skill orchestration.
- **Don't dictate cross-system evidence order**: a tool's companion skill constrains that tool's safety boundary, invocation, and result reading; unless the user or the business contract says so, don't fix the query order across databases, infrastructure, APM, or audit logs.
- **Control user-facing output**: the CLI returns structured detail to the agent; the skill reports only business conclusions, targets/versions, real states, and next steps to the user — never dumps commands, internal IDs, JSON, or irrelevant intermediate steps.

### Companion skill and high-risk writes

When a CLI mutates config, publishes, migrates, deletes, or triggers external execution, the companion skill owns routing and orchestration only — **never the CLI's safety boundary**. Models may read stale docs, invoke stale binaries, or skip a prose rule; wrong operations must be refused by the CLI itself.

1. **Pin the binary before any write**: the skill's first step verifies the actual entry point and required capabilities; when capabilities fall short, build or install a pinned version and keep its absolute path as the session's only entry point. Never mix bare command names with different copies afterwards.
2. **Existing mappings are immutable by default**: a create command hitting a same-name, different-content mapping must return a conflict; only explicit `--replace`/`--force` may overwrite. Before overwriting, the CLI verifies the target's stable identity (repo URL, resource ID, version, or fingerprint) — never let the agent "write first, check later" with defaults or placeholders.
3. **Write target identity into the immutable plan**: high-risk commands plan first; the plan records at minimum target, environment, source version/commit, critical config fingerprints, and creation time. The submit command accepts only that plan and refuses on target or version drift. Never leave the preflight result living only in the agent's prose context.
4. **Keep phases mutually exclusive and distinguishable**: offline `--dry-run`, remote read-only `--preflight`, and real `--apply`/`--yes` are separate paths with distinct output states. Dry-run must not claim remote validation is done; preflight must not produce business writes; show the exact immutable plan before submitting.
5. **Versions are objects, not prose**: when the user names a branch, tag, build number, or commit, the CLI resolves it read-only to the real immutable version and reuses it across the whole batch. Never silently switch version identifiers just because "the current branch happens to point at the same commit".
6. **Expose safety preconditions as machine-readable capability probes**: ship `version`/`capabilities`/`doctor --json` reporting the current binary version, available features, config state, and secret-free diagnostics. The skill branches on that output instead of parsing help prose or guessing PATH; when no usable binary exists, install from the trusted source in this section, then probe again.
7. **Skill write discipline**: the skill may add or replace local mappings only after the user has authorized the high-risk action and the CLI has verified target identity. When automatic confirmation is impossible, leave the config untouched and report the missing business information; never derive a write from a failed anonymous query or a default config.
8. **Prefer read-before-write over `--yes` for updates/deletes**: for changes and deletions of existing resources, force **read-then-write** instead of "confirm to proceed" `--yes` — write commands require `--version` (a resource fingerprint from a read command); the CLI re-reads the target, recomputes the fingerprint, and compares before writing: missing → refuse and prompt to read first (`confirmation_required` / usage exit); mismatch → refuse and demand a fresh read (`conflict` / conflict exit); match → proceed. It forces the agent to read before writing, blocking forgotten context, reckless operations, and **wrong-target operations** (a mistyped app/id naturally fails the fingerprint); the old value stays in session history for review and rollback. **Creates are exempt**: nothing old to read — use duplicate checks instead. On success echo the **newest version token** so chained writes reuse it with no read in between. Fingerprint recipe and algorithm-contract rules: see design-principles.md P1 §5.

### Acceptance

Per verification.md, paste the **actual command and its real output** as evidence for every item — no "confirmed" self-attestation. Both contract compliance (checklist) and smooth real-agent usability (multi-step real tasks + transcript review) are required; evaluation must use an **independent headless agent** (a fresh single-shot/headless run in the tool at hand), never the dev session doubling as tester — the dev session knows every design detail and cannot tell whether prompts and errors are self-explanatory. **Every new or modified (including regression) CLI/companion skill gets this independent-context evaluation**, not just first-time acceptance. High-risk CLIs additionally cover adversarial cases: stale binaries lacking capabilities, same-name mapping overwrites, default/placeholder mappings, version-vs-commit mismatch, mixed phase flags, plan drift, and missing credentials.

## Contract evolution

The JSON envelope, exit codes, and published field names are public contracts — after release, **only add, never change or remove**. Breaking changes bump the interface version and get prominent callouts.
