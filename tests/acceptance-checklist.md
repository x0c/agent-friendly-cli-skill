# Pre-registered acceptance criteria (do not modify after the run starts)

A fresh-machine agent working purely from the skill must find an in-skill basis for every criterion below (citing the file and section), and the plan must **never** contain a dead reference of the form "needs a file outside the skill to continue". Scoring: ✅ basis found and correct / ⚠️ mentioned but missing a key parameter / ❌ missing or decided wrongly on its own.

## A. Self-update and distribution (8 items, all must be self-contained inside the skill)

| # | Criterion | Expected source |
|---|---|---|
| A1 | Update predicate = version first then hash, **forward-only** (prevents stale remote artifacts rolling the machine back), not pure hash alignment | distribution.md |
| A2 | An explicit downgrade-escape environment variable for deliberate rollbacks | distribution.md |
| A3 | Throttling: success 24h, **failures only a short ~1h backoff** (never a 24h failure ban) | distribution.md |
| A4 | Updates run in a separate subprocess; never wired into test entries | distribution.md |
| A5 | Silent: no `--json` stdout pollution; status written under the user cache directory | distribution.md |
| A6 | Kill switches (global + tool-specific + auto under CI) | distribution.md |
| A7 | Real version descriptions injected at build (git-describe style); hardcoded `v0.1.0` forbidden; a post-build `version --json` self-check (the silent-failure lesson) | distribution.md + go-engineering.md |
| A8 | The manifest (sha256 + version + build commit) commits with the artifacts; source changes force an artifact rebuild; a mechanical pre-push check (script + hook, one source) | distribution.md |

## B. Read-before-write fingerprint recipe

| # | Criterion | Expected source |
|---|---|---|
| B1 | Fingerprint = canonical JSON (sorted keys) + the **first 12 chars** of sha256 — not "some chars" | design-principles.md P1 §5 |
| B2 | Exclusion lists fail both ways (over-excluding → guard defeated; missing a volatile field → always conflicts) | design-principles.md P1 §5 |
| B3 | The algorithm is a public contract (changing it invalidates historical tokens; migrate only with a cross-test proving identical values) | design-principles.md P1 §5 |
| B4 | Post-write echo of the newest token; chained writes reuse the previous token | design-principles.md P1 §5 |
| B5 | Creates are exempt and use duplicate checks | implementation-workflow.md high-risk writes #8 |

## C. Credential depth

| # | Criterion | Expected source |
|---|---|---|
| C1 | Directory 0700, file 0600, **read-side** permission and symlink checks, temp-file + atomic rename | implementation-workflow.md ask-once |
| C2 | The three-step password chain (env var → hidden input → non-TTY sentinel error) | implementation-workflow.md |
| C3 | NEED_LOGIN guidance carries the full "ask once" wording and copy-pasteable commands | implementation-workflow.md |
| C4 | A note that cookie sessions and token sessions are not isomorphic | implementation-workflow.md |

## D. The closure loop

| # | Criterion | Expected source |
|---|---|---|
| D1 | Overwrite **every** local install entry (`command -v`, one by one), not just one | distribution.md closure loop |
| D2 | The closure includes: tests green, artifacts + manifest rebuilt and committed, docs/companion skill synced | distribution.md closure loop |
| D3 | The report separates "paths verified by real runs" from "paths covered by static checks/unit tests only" | distribution.md closure loop |

## E. Engineering

| # | Criterion | Expected source |
|---|---|---|
| E1 | Unix-only syscalls split into `_unix.go`/`_windows.go` + `//go:build` | go-engineering.md |
| E2 | The six-platform cross-compilation matrix + artifact naming | go-engineering.md |
| E3 | Extract shared packages when the second CLI appears; copying forbidden; "migrate only what is isomorphic" | go-engineering.md |

## F. Structure and routing

| # | Criterion | Expected source |
|---|---|---|
| F1 | Zero dead references of the form "needs a repo/file outside the skill" | all files |
| F2 | Reading SKILL.md routes correctly to the right reference (self-update → distribution, fingerprint → design-principles, …) | SKILL.md |
| F3 | No topic is long-form duplicated in two places with conflicting wording | all files |

## Pass line

- No ❌ in groups A/B/C/D/E, and ⚠️ ≤ 3 overall (group A ⚠️ ≤ 1);
- F1/F2 must both be ✅ (dead references and routing failures are structural regressions — zero tolerance).
