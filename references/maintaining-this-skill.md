# Maintaining This Skill

Read this when updating this skill itself — distilling new experience into it, deciding what enters and where, auditing its coverage, or feeding back findings from reviews and session audits. This skill grew through repeated incident-driven expansions; this file exists so each maintenance session stops re-inventing its own method (two early maintenance sessions each improvised a high-quality method exactly once, and the third started from zero again).

## When to maintain (triggers)

- After any real session audit or incident post-mortem of a CLI built under this skill.
- Proactively, once a CLI has shipped and accumulated real agent usage — re-read session samples; do not wait for the owner to ask (every historical expansion was owner-initiated; the undistilled backlog exists precisely because of that).
- On a periodic distillation audit (coverage matrix against the living repos).
- When a review could not check something because this skill lacks the clause — that is a skill finding, see "Review-side backfeed" below.

## Source-of-material checklist (mine all of these, every time)

1. This skill's own repo docs (README, repo AGENTS.md).
2. Consuming repos' project instructions and docs — including subproject docs, not only the root.
3. **Git commit history of consuming repos** — fix commits carry lessons that never reached any doc.
4. **Session histories** — the highest-yield source: real call/result pairs, friction signals, recovery trails (methodology: verification.md's session-audit section).
5. **Remote clones/forks of this skill** — other machines may hold undistilled updates; merge before any push.

A document-driven-only audit once missed every lesson that a single session replay surfaced a week later — do not skip sources 3 and 4.

## What enters (A/B criteria)

- **A — do not migrate**: generic engineering common sense any qualified practitioner already holds (envelopes exist, dry-run exists, use test doubles). Restating it bloats the skill without changing anyone's behavior.
- **B — migrate**: incident-grade, transferable, hard-won lessons. Each entry needs the root cause plus a concrete preplay of how the next project hits the same trap. Organize as pitfall → consequence → rule, with the incident narrative kept generic (no company, platform, or host names — generic references and dates only).
- Project-specific contract details (endpoint tables, platform names, local conventions) stay in the consuming repo's docs, never here. One fact, one home: knowledge that already lives in a consuming repo's standards gets a pointer at most, not a copy — and a consuming repo documenting its adoption of a skill-mandated convention is the expected end state, not a signal to duplicate it here.
- **Re-encountered source material**: a raw note that retells the incident behind an existing entry is confirmation, not work. Refresh the entry only when the new material adds an actionable lesson the entry lacks; otherwise cite the covering entry and close with no action.

## Landing map

| Knowledge shape | Destination |
|---|---|
| Philosophy / judging stance | design-principles.md, core philosophy |
| Contract clauses (P0/P1/P2 tiers) | design-principles.md, matching tier |
| Single-point incidents | pitfalls.md (pitfall → consequence → rule, dated) |
| Distribution / build lifecycle | distribution.md, go-engineering.md |
| Acceptance / audit methodology | verification.md |
| Workflow steps / sink-down signals | implementation-workflow.md |
| Non-CLI knowledge | promote to its proper home — repo discipline to the repo's AGENTS.md, platform facts to project docs — never into this skill |

## Owner sign-off boundary

Decisions that change this skill's **product shape** — language, structure, file organization, what the skill is for — are product decisions: present options and wait for the owner's explicit, unambiguous approval. A vague acknowledgment ("ok", "fine") to one part is not approval of the whole package (a real maintenance session once read "ok" as full approval of a restructure + language conversion and had to be reverted).

## The audit → backfeed loop (standard close of every audit)

1. Generalizable lessons → this skill, via the landing map.
2. Project-specific details → the consuming repo's docs.
3. Unimplemented prescriptions → registered as backlog with the owner, not silently dropped.
4. Fix effectiveness → verified at the next audit with a before/after boundary comparison around the fix commit (see verification.md).

An audit that only fixes code and feeds nothing back to this skill has delivered half its value.

## Review-side backfeed

Reviews judge CLIs against this skill as baseline — which makes reviews the earliest place baseline gaps surface ("nothing in the baseline covers this class of problem"). A reviewer who could not check an item because the baseline lacks the clause must report that as a **finding about this skill**, alongside the findings about the CLI under review; feed it through the landing map. Reviews historically consumed the baseline without ever writing gaps back — that channel is this section.

## After every change

Re-run the fixtures in tests/ (fresh-machine simulation per their README). A skill edit that was not fixture-validated is not done. Then sync distribution per the repo's release conventions (merge remote before push; never blind-push).
