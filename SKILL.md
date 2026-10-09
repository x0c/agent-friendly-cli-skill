---
name: agent-friendly-cli
description: "Design or review CLIs and companion skills for reliable agent use, including output contracts, authentication, transactions and acceptance."
---

# Agent-Friendly CLI Development

## Scope

This skill governs **cross-language CLI contract design and acceptance**: how to slice commands, shape output, rank exit codes, handle non-interactive use, avoid pitfalls, and run acceptance. Language-level implementation details (e.g. Go's cobra/flag, Python's argparse) belong to language-specific skills; this skill does not repeat them.

One-line goal: make the CLI **low-token, low-ambiguity, low-risk, auditable, reproducible, and reversible** for agents — and **readable and interactive by default** for humans. One CLI, two audiences — not two tools.

## Which reference to read when

- Design the command surface / output contract / exit codes, or review whether a CLI is agent-friendly → read [references/design-principles.md](references/design-principles.md) (P0/P1/P2 tiers + dual-audience rules). When reviewing, check every item; each gap is a finding to report.
- Implement, and want to dodge real-world incidents → read [references/pitfalls.md](references/pitfalls.md) (dry-run must be truly read-only, idempotency signals, secret safety, test isolation, and other hard lessons).
- Finish building, ready for acceptance → read [references/verification.md](references/verification.md) (evidence-pasting acceptance checklist + real-agent evaluation methodology).

## Selected workflow

Read the applicable design, implementation or acceptance sections of [implementation workflow](references/implementation-workflow.md), then the relevant references listed above. A design review does not start installation, login or production writes. Keep transaction, authorization and terminal-state boundaries for execution tasks.
