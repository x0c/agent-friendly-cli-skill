---
name: agent-friendly-cli
description: "Design or review CLIs and companion skills for reliable agent use, including output contracts, authentication, transactions, distribution and self-update, and acceptance. Also covers auditing real agent sessions for CLI friction after launch, and maintaining/updating this skill itself."
---

# Agent-Friendly CLI Development

## Scope

This skill governs **cross-language CLI contract design and acceptance**: how to slice commands, shape output, rank exit codes, handle non-interactive use, avoid pitfalls, distribute and self-update, and run acceptance. Language-level implementation details (e.g. Go's cobra/flag, Python's argparse) belong to language-specific references (Go: [references/go-engineering.md](references/go-engineering.md)); the main line does not repeat them.

One-line goal: make the CLI **low-token, low-ambiguity, low-risk, auditable, reproducible, and reversible** for agents — and **readable and interactive by default** for humans. One CLI, two audiences — not two tools.

## Which reference to read when

- Design the command surface / output contract / exit codes, or review whether a CLI is agent-friendly → read [references/design-principles.md](references/design-principles.md) — the P0/P1/P2 tiers, dual-audience rules, the read-before-write fingerprint recipe, and contract-evolution red lines. When reviewing, check every item; each gap is a finding to report.
- Build it, and want the full workflow → read [references/implementation-workflow.md](references/implementation-workflow.md) — the P0 checklist, self-serve installation, ask-once login credentials, transaction skeleton, high-risk writes, and acceptance.
- Implement, and want to dodge real-world incidents → read [references/pitfalls.md](references/pitfalls.md) — 46 field-tested entries (dry-run must be truly read-only, idempotency signals, secret safety, test isolation, pagination traps, ecosystem muscle memory, …), each as pitfall → consequence → rule.
- Distribute the CLI → read [references/distribution.md](references/distribution.md) — the minimal complete background self-update spec (forward-only version guard, throttling, kill switches), prebuilt-artifact and manifest discipline, version injection, the post-change closure loop, and companion-skill reverse sync.
- Work in Go → read [references/go-engineering.md](references/go-engineering.md) — the cross-compilation platform matrix, syscall platform split, ldflags version injection, and shared-package reuse rules. A runnable Go reference implementation of these contracts is bundled in this repo root (`cli-common/`, `autoupdate/`, `cli-skeleton/`) — see the bundled-implementation section there before writing your own.
- Finish building, ready for acceptance — or reviewing how a shipped CLI performs in real agent sessions (friction audit, usage review) → read [references/verification.md](references/verification.md) — the evidence-pasting acceptance checklist, real-agent evaluation methodology, and the post-launch session-history audit.
- Update this skill itself — distilling new experience into it, deciding what belongs and where → read [references/maintaining-this-skill.md](references/maintaining-this-skill.md) — maintenance triggers, the five material sources, enter/leave criteria, the landing map, and the audit-backfeed loop.

## Selected workflow

Read the applicable design, implementation, or acceptance sections of [implementation workflow](references/implementation-workflow.md), then the relevant references listed above. A design review does not start installation, login, or production writes. Keep transaction, authorization, and terminal-state boundaries for execution tasks.

## Contract evolution

The JSON envelope, exit codes, and published field names are public contracts: after a version ships, **only add, never change or remove**. Breaking changes bump the interface version and get prominent callouts. Details at the end of design-principles.md.
