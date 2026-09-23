# agent-friendly-cli

A skill for designing command-line tools that AI agents can call reliably — without making them worse for the humans who still use them. Guidance plus review checklists. Not a CLI binary, not a framework.

## The philosophy

Everything in this skill follows from four ideas.

### 1. One CLI, two audiences

Traditional CLIs are built for humans: they use color, ask `[y/N]`, and expect you to guess what an error means. An AI agent driving that same command line can't see color, hangs forever on a confirmation prompt, and burns tokens — or misreads results — on a raw text dump.

So the skill designs for two audiences at once, in a single tool:

- **For agents:** low token, low ambiguity, low risk — plus auditable, reproducible, and reversible.
- **For humans:** readable and interactive by default, with no need to memorize the machine contract.

The machine contract appears on demand (`--json`, or automatically off-TTY) and never at the human's expense. An interactive wizard always has a non-interactive equivalent. One CLI, two faces — not two tools.

### 2. Contracts, not prose

Agents cannot reliably parse prose, and prose drifts from behavior. So every judgment an agent must make gets a machine-checkable surface:

- Success and failure share one envelope, so errors are *seen* as data instead of swallowed as exceptions.
- Exit codes map to the tool's real failure modes, so callers branch without parsing text.
- `describe` output shares one definition with the argument parser, so docs can never lie about flags.
- Errors carry a stable code, a hint, and the literal next commands — the failure tells the agent how to recover.
- Output separates *coverage* (what was actually read) from *results* (what matched), so "zero hits" never masquerades as "everything is clean."

If a fact matters to the agent, it is structured. Prose is presentation layered on top — never the source of truth.

### 3. Safety lives in the CLI, not in the docs

Models skip written rules, read stale guides, and invoke stale binaries. Any protection that exists only as a sentence will eventually be walked past. So everything that prevents harm is enforced by the CLI itself, which refuses the wrong operation even when asked:

- Updates and deletes require reading first — a fingerprint taken at read time is re-checked at write time, which also stops wrong-target operations.
- High-risk work plans first: target identity goes into an immutable plan, and the submit step accepts only that plan.
- Rehearsal, remote pre-checks, and real execution are separate, mutually exclusive phases — a dry run can never claim what only a preflight knows.
- Existing mappings are immutable by default; overwrites need an explicit flag.

The companion skill routes, orchestrates, authorizes, and explains — it is never the safety boundary. The default flow is a transaction skeleton: probe capability → resolve the target from evidence → write an immutable plan → rehearse offline → pre-check remotely → authorize once by risk → apply → prove the terminal state. Steps may be trimmed by risk, but the order never inverts, and done always means *converged*, not merely *exited zero*.

### 4. Done means converged — and proven

A second run of a write must settle to `unchanged`. A write's success is judged by re-reading the target's state, not by the submit acknowledgment. And acceptance is never self-attested: every checklist item pastes the actual command and its real output as evidence, and a fresh headless agent — one that never saw the design discussion — must complete a real multi-step task while its raw transcript is reviewed for stuck points. If only the dev session can operate the tool, the tool isn't finished.

## Start here

`SKILL.md` is the entry point: scope, the hard requirements, and which reference to read when. `references/` holds the full standard, the field pitfalls, and the acceptance checklist, loaded on demand.

[MIT](LICENSE)
