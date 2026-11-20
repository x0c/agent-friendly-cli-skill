# Test run log

## 2026-10-29 — run 1 (Chinese body, before the English-only policy)

- Fixtures: mcfoo-brief.md; executor: an independent-context agent (skill directory only; tests/ and every internal repo forbidden), bullet-style output, lowered thinking level.
- Result: **pass**. A-group 8/8 (forward-only / escape hatch / throttle / subprocess / silent / kill switches / version-injection self-check / manifest + pre-push gate all hit); B 4✅+1⚠️; C 4/4; D 3/3; E 2✅+1⚠️; F 3/3 (zero dead references, routing correct).
- The two ⚠️ (B3 fingerprint-contract line not restated; E3 shared-package red line not expanded) were verified present in the skill — artifacts of the compressed bullet output, not coverage gaps; no backfill.
- Baseline: the same simulation against the pre-fix skill (full-plan output) produced 27 "own calls"; this run reduced them to 5, all outside skill scope (platform API, domain fields, library name, repo name).
- Execution note: the evaluation agent's first run at the default thinking level hit "run hit the output token limit before producing any text" (thinking budget consumed the output); lowering the thinking level finished it in 51 seconds.

## 2026-10-29 — run 2 (English body, after the English-only policy)

- Same fixtures (English brief), same constraints, bullet-style output, lowered thinking level.
- Result: **pass with zero warnings**. A 8/8, B 5/5, C 4/4, D 3/3, E 3/3, F 3/3 — every criterion carried an in-skill citation; no dead references; routing correct.
- Residual "own calls": 7, all outside skill scope (concrete credential path, env-var names, widget domain model, first-CLI package layout, session type pending endpoint observation, human locale, git hosting).

## 2026-11-20 — run 3 (design brief, after the batch-1 expansion)

- Fixtures: mcfoo-brief.md extended with two new trap surfaces (`widget deploy` minutes-long convergence; kubectl-habituated colleagues' agents). Executor: independent-context agent (skill directory only, tests/ forbidden).
- Result: **pass**. A 8/8 (subprocess/silent/forward-only/escape/throttle/kill-switches/version-injection self-check/manifest+gate); B 5/5; C 4/4; D 3/3; E 3/3; F 3/3 (zero dead references, routing correct); G 4✅ + 1⚠️.
- The new entries are findable and applicable by a fresh agent: §10/§11 carried correct in-skill citations for ecosystem aliases (incl. help-path resolution), usage-error envelopes with raw-argv scan, both-mode wait primitive (transient-failure tolerance, multi-target, root-cause-extraction follow-up), and the angle-bracket rule.
- The one ⚠️ (G5: enumeration-completeness over the valid universe incl. synthetic entities was not restated) — the clause is present in design-principles P1 §8 and the agent cited other sub-points of that same item; classified as a restatement miss, no backfill; to be re-tested by the next real-platform blind runs (three sites, same fresh-agent setup).

## 2026-11-20 — run 4 (maintenance brief, new fixture)

- Fixtures: maintenance-brief.md (six raw notes: 2 already-covered, 2 not-yet-covered, 1 review-side baseline gap, 1 owner-boundary). Executor: independent-context agent.
- Result: **pass, H 5/5**. Correctly recognized the muscle-memory pitfall entry, distribution.md §1 + one-fact-one-home, the shared-error-helpers pitfall paired with the Review-side backfeed section, and the Owner sign-off boundary (citing the vague-"ok" history verbatim). Correctly routed the two genuine gaps: cached platform-owned binding as write-target source → pitfalls.md; acceptance-script precheck failure/skip semantics → verification.md.
- Backfill applied from its "skill silent" notes: maintaining-this-skill.md gained the **re-encountered source material** rule (confirmation, not work — refresh only when the new material adds an actionable lesson the entry lacks) and the **adopted-convention end state** clarification.
- Independent cross-validation: the two gaps it flagged on its own match the already-planned batch-2/batch-3 backlog items (identity-mapping drift, precheck-failure semantics) — different agents reaching the same gaps from different paths.

## 2026-11-20 — run 5 (three real-platform blind designs, user-directed extension)

- Setup: three independent-context agents, one real internal platform each (a CICD/release platform, a task-scheduling platform, a message-bus platform) — each given only the site URL + a business-capability paragraph + this skill; forbidden every existing CLI/tool/repo/config for that platform; unauthenticated read-only GETs permitted. Same pre-registered criteria as run 3 (no criteria changes). Plans: /tmp/pubcli-eval-{1,2,3}/plan.md.
- Results: **all three pass**. Pub: A 8/8, B 5/5, C 4/4, D 3/3, E 3/3, F 3/3, G 4✅+1⚠️ (no synthetic-entity surface for G5). TSP: **zero warnings** (B2 both-ways and G5 completeness both restated; correct applicability judgment — no dominant ecosystem tool in the scheduling domain, absorb generic Unix habits only). MBP: A 8/8, B 4✅+1⚠️, C-E 3/3 each, F 3/3, G 5/5 (ecosystem anchor correctly identified as rabbitmqctl; explicit no-wait-needed finding with justification).
- Cross-run determinism: contract layers (envelope, exit codes, write guard, credentials, self-update, closure) near-identical across all three; divergence only in domain-adaptive choices (ecosystem anchor, negative scope decisions) — the healthy kind.
- Reference-CLI comparison: the pub blind design reproduced, from the skill alone, every friction-driven design the real CLI for that platform had to learn over months (multi-target wait primitive, root-cause extraction view, single business-code judgment layer, window/queue preflight). The one real-CLI capability no blind design produced is search-input separator normalization with auto-retry — matching the already-planned batch-2 backlog item.
- G5 concern from run 3 resolved: picked up by 2 of 3 blind agents where the platform surface allowed it; no backfill.
