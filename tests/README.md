# tests/: skill regression fixtures

After changing the skill (SKILL.md or any reference), run a "fresh-machine agent" regression with the fixtures in this directory to verify the skill is self-sufficient with **zero internal-repo context**.

## Why

One of this skill's audiences is "an agent on another machine with no context from our repos". The dev session already knows every design detail and cannot test whether the skill is self-explanatory — only a fresh-context agent working purely from the skill can.

## How to run

1. Start an agent with **independent context** (requires explicit authorization from the environment/user; without it, run the multi-step path directly from the current session and report honestly that "independent usability was not evaluated").
2. Give it exactly two things: the absolute path of this skill directory + the task brief in [mcfoo-brief.md](mcfoo-brief.md). Explicitly forbid reading any internal CLI repo or internal standards file.
3. Take back its plan and score it item by item against [acceptance-checklist.md](acceptance-checklist.md).
4. On any failure, first classify "the skill doesn't say" vs "it says but wasn't found" (a routing failure), patch the corresponding file, then re-run the failed items.

## Criteria maintenance discipline

acceptance-checklist.md holds **pre-registered criteria** — never adjust them after seeing results; criteria changes (additions/removals) must happen before a run and record the reason.
