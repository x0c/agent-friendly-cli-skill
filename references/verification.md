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

The checklist proves "contract compliance"; real-agent evaluation proves "agents can actually use it smoothly" — do both.

- **Run multi-step tasks with a real agent**, not a human review of the design doc. A task with weight usually takes dozens of tool calls to surface design problems.
- **Independent context is mandatory**: the test agent starts fresh through the tool's headless/single-shot mode (with that tool's permission-skipping flags), receives only the task brief and the tool name — no dev-session context shared, no design pre-briefing. Dev-session self-tests don't count: they know every convention and can't tell whether hints/next_commands are self-explanatory. The brief tells the agent "recover from errors via their hints, don't ask the user", and asks it to summarize which steps intercepted it and how it recovered — interception → self-recovery is exactly the acceptance point for read-before-write, capability probes, and friends. Test data gets really verified and cleaned up.
- **Collect metrics**: task success rate, tool-call rounds to finish, token cost, error rate, plus call fan-out over identical target/keyword combinations. When a stable N×M serial pattern appears, prefer adding a CLI batch primitive over rewording prompts.
- **Read the raw transcript, never just the agent's summary** — agents smooth over their stuck points; only the transcript shows which command output confused them and which interaction point took many retries.
- **Close the loop**: hand the transcript to a different agent for analysis, let it name the CLI's ambiguity points, and iterate on that.

Evaluation can ship as part of the CLI (e.g. `tool selftest`) and regress on every change instead of living as a one-off external script. Landing convention: the repo carries a **side-effect-free-by-default** contract assertion script (e.g. `scripts/test_agent_friendly.sh`, asserting envelope/exit codes/non-interactive/describe offline), with real read-only integration behind an explicit env-var opt-in (e.g. `*_LIVE_*`) and write acceptance only in test environments against self-created objects — so the script runs in CI on every change without polluting the real environment.
