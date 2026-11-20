# Task brief: distill raw incident notes into the agent-friendly-cli skill

You are the maintainer of the skill at `<absolute skill directory path>`. Using only files inside that skill directory (SKILL.md and every file it references), decide for each raw note below:

- does it enter the skill? If yes: which file, what kind of entry, and a one-line summary of the entry;
- is it **already covered**? If yes: cite the file and section that covers it — no new entry;
- does it belong **outside** the skill? If yes: where, and why;
- is it a **shape decision** (changes the skill's language/structure/organization)? If yes: state what approval it needs.

**Constraints**: you may read only files inside that skill directory; do not read, search, or assume the existence of any other internal repository, standards file, or machine-local config. For every decision beyond the skill, mark it "skill silent → my own call".

## Raw notes (post-mortem material from various CLI projects, 2026-11)

1. A 60-day session audit of a Kubernetes-platform CLI (2,500+ calls): 76% of failing sessions had read the companion skill. Agents typed `pods`, `log`, `deployment`, `cluster list` — none of these commands exist; the framework's closest-match suggestion sits in the error message while the hint says "check arguments, profile configuration, and network connectivity". Auditors rewrote the companion-skill wording; the miss rate did not move. (The CLI later registered subcommand aliases and the misses went to zero.)

2. During a release, a CLI trusted a locally cached project→repository mapping over the platform's live binding field; the mapping had drifted, the release tag went to the wrong repository, and delivery failed downstream. The fix reads the platform field as the authoritative source before every write and hard-fails on drift.

3. A repo's AGENTS.md documents that its pre-push hook reruns the artifact-manifest check so that stale binaries cannot be pushed.

4. A verification script's database precheck failed silently and the script still reported PASS; 17 green checks could not prove any DB assertion had actually run. The project fixed it by making precheck failure FAIL or an explicit SKIP, with an explicit `--no-db` downgrade flag.

5. A five-dimension code review judged a CLI's write-safety "no violations"; a session replay 73 days later found an error path steering agents into a destructive command. The review's baseline contained no clause about error-hint command context, so no reviewer ever checked that class of problem — and the reviewers noted nothing about the baseline gap either.

6. A maintainer proposes converting all skill files to a tab-indented, numbered-section style "for consistency", and asks the owner to approve. In the same chat, the owner replied "ok" to a different, unrelated question.

## Output format

For each note: verdict (enter / already covered / stays outside / shape decision) → destination + entry type (or the covering citation) → one-line basis ("which skill file and section says so" or "skill silent → my own call"). Then one closing section: "notes the skill did not give me a rule for".
