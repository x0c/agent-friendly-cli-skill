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
