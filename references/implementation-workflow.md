# Implementation Workflow: Design through Acceptance

Follow this file end-to-end when building or retrofitting a CLI. For pure review work, read [design-principles.md](design-principles.md) directly and skip the workflow.

## Design phase

Clear the **seven P0 hard requirements** first — without them agents cannot use the tool at all. Details in design-principles.md:

| P0 | In one line |
|---|---|
| Non-interactive mode | Auto-disable interaction when non-TTY is detected (`isatty()`, stdin/stdout tested separately); never require the caller to pass a flag |
| Structured output | `--json` with a uniform envelope `{ok, data, error, meta}`; failures also go through stdout JSON + a non-zero exit (human/pipeline-first tools may ship bare-data JSON instead — pick one, never mix; see the reference) |
| Layered exit codes | 0 success / 1 generic failure / 2 usage error / 3 not found / 4 auth / 5 conflict / 6 timeout (map to this tool's real failure modes — see the reference) |
| Dry-run | Side-effecting commands support rehearsal with the same output shape as real execution; truly read-only, costs nothing |
| Verification commands | `status`/`verify`/`doctor` so callers can double-check beyond the exit code |
| Input validation | Hard-block path traversal and command injection |
| Self-serve installation | When the companion skill cannot find the CLI, give a trusted download source, install into the user directory automatically, then verify version and capabilities |

Then add P1 as needed (`describe` self-description, structured errors with `hint`/`next_commands`, size control, write-ahead log, auto-generated SKILL.md, composability). Prefer **declarative commands** (`ensure`/`apply`) over imperative ones (`create`/`delete`); they are idempotent by nature.

**Dual audience** throughout: human-readable by default (tables and color allowed); strip everything down to the machine contract under `--json` or non-TTY. Every interactive wizard needs a non-interactive equivalent (`--yes` + full flags). Give dual fields for the same fact (stable English enum `status` for programs, localized `status_tag` for humans).

## CLI availability and self-serve installation

The companion skill's first step must probe the CLI's absolute path and `version`/`capabilities`. When no usable binary is found, do not reply "CLI not installed" or push the installation back to the user; a user request for that capability authorizes the agent to finish a **privilege-free, user-directory-only** local install and continue the original task — unless the user explicitly said not to install.

- **Machine-executable source**: state the trusted official repo or per-platform artifact download URL in the skill, the CLI-generated skill template, or `describe` output. Never give only a product homepage, make the agent search for a download link, or rely on an unstable package-manager name.
- **Deterministic install path**: ship a non-interactive install script or equivalent command that picks a prebuilt artifact by OS/CPU, installs into a user-writable directory, uses no `sudo`, and touches no system directories or shell configuration. When an artifact already exists at a different version, report a conflict by default; overwrite only with explicit `--force`/`--replace`.
- **Private distribution**: when the trusted source is private, the installer reads its token from an environment variable or the OS credential store (never a prompt, never printed) and keeps a second trusted path — a source build of the same pinned tag over the same private Git host — for machines without the token. Both paths verify what they installed.
- **Pin and verify**: after installing, record the absolute binary path, download source, and immutable version/commit; immediately run `version --json`, `capabilities --json`, or `doctor --json` to verify, then use only that absolute path for the rest of the session. Verify checksums or signatures whenever provided.
- **Explicit failure boundary**: only report a concrete blocker when the download source is unreachable, no artifact exists for the platform, verification fails, or a required tool (git/downloader) is missing. Never use a random URL from search results, a low-level API, or an ad-hoc `curl` to bypass the CLI. A CLI with no trusted download source and no executable install path is not a complete agent-usable deliverable.
- **Background self-update (when distributed from the same source)**: if the CLI is distributed long-term from an official repo's prebuilt-artifact directory, installs should also carry **silent background self-update**. The minimal complete spec (forward-only guard, version-then-hash predicate, throttling, kill switches, separate process) lives in [distribution.md](distribution.md).
- **Companion-skill reverse sync (when a companion skill exists)**: the install path solves "guide the agent to install when the CLI is missing", but the reverse link needs managing too — agents on machines where the skill was installed never learn about skill updates. The CLI runs the skill manager's own update command silently in the background instead of reimplementing per-agent directory sync. Eligibility, the authoritative-machine exemption, mutual exclusion, and throttling conventions live in [distribution.md](distribution.md).

## Ask-once login credentials

Once a CLI persists account credentials locally and auto re-logs in on session expiry, the companion guide must make the agent behave as "ask the user **once**, then never again". With the capability present but that one sentence missing from the guide, every new session degrades into asking for the password again (real incident: a registry CLI's guide only said "provide the password via environment variable", and another agent pushed setting the variable back to the user).

- CLI side: the login command accepts the password via environment variable (never blocks on interaction when non-TTY); credentials/profiles persist account and password together; expired sessions re-login automatically and refresh what is on disk.
- **Credential storage depth**: the directory sits at exactly `0700`, credential files at `0600`; the **read side** validates permissions and symlinks too (against downgrade attacks and path swaps); writes go through a temp file + atomic rename, never in-place; the password read is a three-step chain — environment variable → hidden terminal input → a structured sentinel error when non-TTY and unset (mapped to a NEED_LOGIN-class error, never an interactive hang).
- Error side: NEED_LOGIN-class error hints must spell out "ask the user once for account + password → run login with env vars → persisted, auto re-login after that", with copy-pasteable env-var commands in next_commands. A bare "please log in again" says nothing.
- Skill side: the login step states "password via environment variable, persisted locally with auto re-login — ask once; do not make the user set env vars themselves or ask repeatedly".
- Upgrade compatibility: old persisted credentials without a password report NEED_LOGIN the same way, with a hint that one fresh login upgrades them to the new format; credential file paths and field names are public contracts, and any format change ships with read/write compatibility tests for the old format (compat_test).
- **Session types do not transfer**: the re-login discipline above is written for cookie/form sessions (401, a 302 to the login page, or login-form HTML are the only session-expiry signals). Token-based sessions (OAuth or self-signed tokens) refresh through a different mechanism — refresh tokens rotate and may be shared with another party; design them separately and follow the "a borrowed login must stay shared, not forked" pitfall.

## Implementation phase

Walk pitfalls.md and dodge each item. CLIs that reuse another tool's login or spend generation quota have dedicated entries there (shared login refresh, spend ledger, no auto-retry after uncertain paid calls, advisory parameters, binary artifacts). Priorities: the dry-run branch must intercept **every** side effect; write commands must converge to `ok` on the second run; secrets come only from environment variables and never enter logs/output/git, with credentials bound to the login-time service origin; upstream business success criteria stay decoupled from HTTP status codes, and write success is judged by re-reading terminal state; re-login happens once and only on session-expiry signals, and write requests with uncertain outcomes are never auto-replayed; tests redirect to temp directories and never touch real user resources.

## Companion-skill layering and transaction skeleton

A companion skill is not a CLI command list — it compiles the user's natural-language goal into an auditable, authorizable, recoverable, verifiable transaction. Write down the business terminal-state invariant first, e.g. "all deploy targets succeeded on the same version" or "platform state strictly matches code". Never define "some command exited 0" as done.

| Layer | Owns | Must not own |
|---|---|---|
| CLI | auth, target identity checks, version resolution, business hard gates, planning/diffing, idempotency, execution, status queries, structured errors | relying on the agent to remember critical safety rules |
| Helper scripts | cross-tool adaptation, legacy CLI compatibility, format conversion, temporary deterministic glue | long-term ownership of high-risk domain rules (pagination, snapshot consistency, deletion order) |
| Skill | intent routing, context lookup, flow orchestration, risk authorization, exception branches, result presentation | reimplementing computations the CLI already does deterministically, or calling low-level APIs to bypass the CLI |
| Agent | genuine semantic ambiguity and user decisions | parsing unstable prose, hand-computing diffs, guessing targets or defaults |

Follow this transaction skeleton by default; trim steps by task risk, never invert the safety order:

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
- **Read sink-down signals**: stable, repeated, mechanical, cross-skill, or high-risk-consistency logic belongs in the CLI as a first-class command. When skills/scripts start owning pagination, diffing, loose matching, snapshot lifecycles, deletion order, or complex state machines long-term, the CLI is usually missing `plan/apply/verify`, `sync`, `batch` composites; scripts may bridge the gap but must not become a second business core.
- **Sink batch reads too**: when the agent repeats the same discovery/diagnostic/filter command over one target set, the CLI should offer a one-call `batch`/`--all-matches`/aggregate command with bounded internal concurrency, per-target results, an overall summary, and partial/failures. Never treat N×M serial tool calls as skill orchestration.
- **Don't dictate cross-system evidence order**: a tool's companion skill constrains that tool's safety boundary, invocation, and result reading; unless the user or the business contract says so, don't fix the query order across databases, infrastructure, APM, or audit logs.
- **Control user-facing output**: the CLI returns structured detail to the agent; the skill reports only business conclusions, targets/versions, real states, and next steps to the user — never dumps commands, internal IDs, JSON, or irrelevant intermediate steps.

## Companion skill and high-risk writes

When a CLI mutates config, publishes, migrates, deletes, or triggers external execution, the companion skill owns routing and orchestration only — **never the CLI's safety boundary**. Models may read stale docs, invoke stale binaries, or skip a prose rule; wrong operations must be refused by the CLI itself.

1. **Pin the binary before any write**: the skill's first step verifies the actual entry point and required capabilities; when capabilities fall short, build or install a pinned version and keep its absolute path as the session's only entry point. Never mix bare command names with different copies afterwards.
2. **Existing mappings are immutable by default**: a create command hitting a same-name, different-content mapping must return a conflict; only explicit `--replace`/`--force` may overwrite. Before overwriting, the CLI verifies the target's stable identity (repo URL, resource ID, version, or fingerprint) — never let the agent "write first, check later" with defaults or placeholders.
3. **Write target identity into the immutable plan**: high-risk commands plan first; the plan records at minimum target, environment, source version/commit, critical config fingerprints, and creation time. The submit command accepts only that plan and refuses on target or version drift. Never leave the preflight result living only in the agent's prose context.
4. **Keep phases mutually exclusive and distinguishable**: offline `--dry-run`, remote read-only `--preflight`, and real `--apply`/`--yes` are separate paths with distinct output states. Dry-run must not claim remote validation is done; preflight must not produce business writes; show the exact immutable plan before submitting.
5. **Versions are objects, not prose**: when the user names a branch, tag, build number, or commit, the CLI resolves it read-only to the real immutable version and reuses it across the whole batch. Never silently switch version identifiers just because "the current branch happens to point at the same commit".
6. **Expose safety preconditions as machine-readable capability probes**: ship `version`/`capabilities`/`doctor --json` reporting the current binary version, available features, config state, and secret-free diagnostics. The skill branches on that output instead of parsing help prose or guessing PATH; when no usable binary exists, install from the trusted source in this section, then probe again.
7. **Skill write discipline**: the skill may add or replace local mappings only after the user has authorized the high-risk action and the CLI has verified target identity. When automatic confirmation is impossible, leave the config untouched and report the missing business information; never derive a write from a failed anonymous query or a default config.
8. **Prefer read-before-write over `--yes` for updates/deletes**: for changes and deletions of existing resources, force **read-then-write** instead of "confirm to proceed" `--yes` — write commands require `--version` (a resource fingerprint from a read command); the CLI re-reads the target, recomputes the fingerprint, and compares before writing: missing → refuse and prompt a read first (`confirmation_required` / usage exit); mismatch → refuse and demand a fresh read (`conflict` / conflict exit); match → proceed. It forces the agent to read before writing, blocking forgotten context, reckless operations, and **wrong-target operations** (a mistyped app/id naturally fails the fingerprint); the old value stays in session history for review and rollback; and it doubles as an optimistic lock. **Creates are exempt**: nothing old to read — use duplicate checks instead. Frame the guard as constraining the agent itself; rejection prose must explain why the write is blocked and `next_commands` carries a copy-pasteable read command with real arguments. On success echo the **newest version token** and the updated object so chained writes reuse it with no read in between. The fingerprint recipe, exclusion lists, and the "algorithm is a public contract" migration red line: see design-principles.md P1 §5.

## Acceptance phase

Per verification.md, paste the **actual command and its real output** as evidence for every item — no "confirmed" self-attestation. Both contract compliance (checklist) and smooth real-agent usability (multi-step real tasks + transcript review) are required; the evaluation must use an agent with **independent context** — the dev session knows every design detail and cannot tell whether prompts and errors are self-explanatory. **Every new or modified (including regression) CLI/companion skill gets this independent-context evaluation**, not just first-time acceptance. High-risk CLIs additionally cover adversarial cases: stale binaries lacking capabilities, same-name mapping overwrites, default/placeholder mappings, version-vs-commit mismatch, mixed phase flags, plan drift, and missing credentials.

Independent-context evaluation needs explicit authorization from the environment or the user: this workflow does not authorize workers, does not override the environment's model and delegation policy, and does not start native subagents on its own. When authorized, start a permitted external runtime fresh and give it only the task brief and the tool name; without authorization, run the real multi-step path directly from the current session and report honestly that "independent usability was not evaluated".

After the CLI has been in production use for a while, run one more round of **real session-history auditing** (method at the end of verification.md): mine this CLI's every call-and-result pair from local agent-runtime session JSONL, locate real pain points via friction signals (failures bucketed by error code, `--help` probes, repeated retries, text output parsed by sed), conclude by **recovery steps** rather than failure counts, and exclude designed safety interceptions (two-stage confirmation, missing read-before-write tokens) from friction; after fixing, re-verify with an independent-context agent that it recovers purely from the error envelope.
