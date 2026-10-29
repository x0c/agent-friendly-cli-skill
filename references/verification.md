# CLI Acceptance: Checklist + Real-Agent Evaluation

Read this after building or retrofitting a CLI. **A principle list never replaces a real run**: no matter how right the design looks, nobody knows whether agents will get stuck until one actually runs.

## Acceptance checklist (paste the actual command + output as evidence per item)

No "confirmed" or "should be fine" self-attestation. Every item shows the command actually run and its real output, with the output proving the conclusion.

1. **Non-interactive never hangs**: run a side-effecting command in a non-TTY environment (strip the TTY with a pipe or redirect, e.g. `echo | tool do-something`) and confirm it downgrades automatically instead of parking on a confirmation prompt. Paste command + exit code.
2. **Success-path envelope compliance**: `tool <cmd> --json` stdout parses directly under `json.loads`/`jq` and carries all four keys `ok/data/error/meta`. Paste the output.
3. **Failure-path envelope compliance**: manufacture a failure (missing argument, nonexistent resource, auth failure) and confirm stdout is still a valid JSON envelope (`ok=false`, `error.code` present) with a non-zero exit. Paste the failure output + exit code.
4. **Exit codes measured per class**: verify at least usage error (2), not-found (3), and auth/permission (4) each return the expected code. Paste `echo $?`.
5. **stdout/stderr separation**: `tool <cmd> --json 2>/dev/null | jq .` succeeds — proving no logs leaked into stdout.
6. **Idempotency convergence**: run a write command twice in a row; the first run reports the write action, the second converges to `ok`/`unchanged`. Paste both outputs.
7. **Dry-run truly read-only**: snapshot the target directory/resources first (`ls -la`, hashes, record counts), run `--dry-run`, snapshot again, and confirm both snapshots are identical. Paste both snapshots.
8. **Offline self-description**: `describe`/`version`/`doctor` (the config-check parts) still print with the server unreachable. Paste the output.
9. **Human-readable by default**: run the same command without `--json` and confirm human-readable output with no raw JSON leaking through. Paste the output.
10. **No secret leakage**: search `doctor`/error output/logs and confirm no secret/token plaintext anywhere.
11. **Missing CLI self-recovers**: in a clean environment without the CLI on PATH, run the companion skill's install branch and verify it uses only the declared trusted repo/artifact URL, installs per current OS/CPU into a temp user directory with no `sudo`, and runs `version`/`capabilities`/`doctor` from the absolute path successfully. Re-installs converge to already-installed or demand explicit overwrite; paste download source, install path, version proof, and exit code.
12. **Parameters really take effect**: pick time-window/filter/scope parameters from `describe` and verify against an observable fixture that they actually enter the request or change the result; then pass an unrelated parameter from another subcommand and confirm exit 2 instead of success-with-ignored-input.
13. **Zero hits stay readable**: construct both "raw reads > 0 but filtered hits = 0" and "raw reads = 0", and confirm `read/scanned`, `matched`, `returned`, actual coverage, truncation/cap state, and partial/failures tell the two apart.
14. **Batch calls converge**: run a batch/aggregate command over at least two targets and confirm one call returns per-target results, an overall summary, and partial/failures; verify bounded concurrency and that one target's failure doesn't swallow the other successes.
15. **Binary identity continues**: run the CLI from its absolute path and check that `hint`/`next_commands` and later skill steps never fall back to a bare PATH command or a different copy; paste the resolved path and version.

Write-path acceptance runs one real minimal end-to-end loop, then cleans up or completes the test data. When the CLI depends on LLMs, search, queues, or middleware, production config goes through the real path for verification — local unit tests alone don't count.

## Real-agent evaluation methodology

The checklist proves contract behavior. A fresh-agent evaluation measures independent usability and is useful when explicitly authorized. This procedure does not authorize workers or override the environment's model and delegation policy; do not start native subagents. Without worker authorization, exercise real multi-step paths directly and report that independent usability was not evaluated.

- **Run multi-step tasks with a real agent**, not a human review of the design doc. A task with weight usually takes dozens of tool calls to surface design problems.
- **Independent context for authorized evaluations**: start the permitted external runtime fresh in its headless/single-shot mode, using the current model and permission policy. Give only the task brief and tool name. Development-context runs establish behavior, not independent discoverability. Ask the evaluator to recover through command hints and identify obstacles; inspect and clean up its test data.
- **Collect metrics**: task success rate, tool-call rounds to finish, token cost, error rate, plus call fan-out over identical target/keyword combinations. When a stable N×M serial pattern appears, prefer adding a CLI batch primitive over rewording prompts.
- **Read the raw transcript, never just the agent's summary** — agents smooth over their stuck points; only the transcript shows which command output confused them and which interaction point took many retries.
- **Close the loop**: inspect the raw transcript for ambiguous CLI behavior and iterate. A separately authorized reviewer may provide an independent assessment; a second agent is not required to complete the work.

Evaluation can ship as part of the CLI (e.g. `tool selftest`) and regress on every change instead of living as a one-off external script. Landing convention: the repo carries a **side-effect-free-by-default** contract assertion script (e.g. `scripts/test_agent_friendly.sh`, asserting envelope/exit codes/non-interactive/describe offline), with real read-only integration behind an explicit env-var opt-in (e.g. `*_LIVE_*`) and write acceptance only in test environments against self-created objects — so the script runs in CI on every change without polluting the real environment.


## Post-launch regression: real session-history audit

Acceptance tells you whether the design *can* be used; a session audit tells you where real usage *hurts*. After the CLI has been in production for a while (especially once agents use it via its companion skill), mine every call-and-result pair for this CLI from local agent-runtime session histories (JSONL) and take stock of friction signals:

1. **Extract**: scan session files for tool calls (bash command lines matching the CLI name, excluding same-named directories/identifiers), pair each with its following result; aggregate by day, command, and arguments.
2. **Classify friction signals** (each class must land on a fixable concrete point):
   - **Failure rate bucketed by `error.code`**: whichever code dominates is the envelope or pre-validation to fix;
   - **`--help` probe counts per command**: a command probed repeatedly means its syntax never made it into the skill or the help lacks examples (agents verifying syntax at point of use is inevitable behavior, not a skill-writing failure);
   - **Exact-duplicate command executions**: separate legitimate polling from failure retries;
   - **Text output followed by sed/grep/awk**: human-readable rendering used as a data channel (see the matching pitfalls entry) — the read command lacks a lossless extraction primitive;
   - **Absolute-path pinned invocations**: the agent distrusts PATH — usually scar tissue from past version drift across install entries; verify all entries run the same version.
3. **Conclude by recovery steps, not failure counts**: for each failed call, count how many calls until success (or giving up); envelopes that recover in one step need no work — multi-step blind guessing is what to fix. Also exclude **designed interceptions** from friction: two-stage confirmations, missing read-before-write tokens and other safety interceptions mandated by the skill are design behavior when they recover in one step — don't "fix" them as bugs (in a real audit, `confirmation_required`/`pod_required` all recovered in one step and were the most likely to be misjudged).
4. **Re-verify with an independent headless agent after fixing**: give it only the task, no hints; watch whether it recovers purely from the error envelope. Record which envelope fields it *actually used* (candidates / next_commands / hint / context items), and where "the error was not enough to recover — I had to guess" — those two sentences are the entire input for the next iteration.
