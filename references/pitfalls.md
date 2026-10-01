# CLI Pitfalls from the Field

Read this while implementing to dodge each pitfall. Every item is "pitfall → consequence → rule". All of them come from real project incidents, not armchair theory.

## Dry-run must be truly read-only

**Pitfall**: the `--dry-run`/`--check` branch still performs writes — files, directories, backups, deletions, copies, renames.
**Consequence**: a so-called "preview" really mutates the user directory; users feel safe precisely when they are being harmed.
**Rule**: the dry-run branch must intercept **every** side-effect path — confirm each write is short-circuited by `if opts.DryRun`. `would create ...`/`would modify ...` lines state intent; they are not executions. Real execution is a separate run without dry-run.

## Dry-run must not spend money or notify

**Pitfall**: `--dry-run` calls a paid model API or sends notifications "to preview more accurately".
**Consequence**: a preview really burns quota and really disturbs users; when quota runs dry the preview itself fails.
**Rule**: dry-run means "no business data written, no notifications, no external quota consumed". When a preview genuinely costs money, name it `preview` or label the cost explicitly in the output — never hide spending inside dry-run. Prefer purely local previews: construct the request body, show the target URL and the identity that would be used, but send no write request.

## Idempotency is an acceptance signal, not just a feature

**Pitfall**: a write command reports "created" on every run; the second run never converges.
**Consequence**: nobody can tell whether the operation took effect, retries are unsafe, and tests have no stable assertion point.
**Rule**: write commands return `created`/`merged`/`applied` the first time and **must converge to `ok`/`unchanged` the second time**. That is both the user's idempotency signal and the best test assertion. Declarative commands satisfy this naturally.

## One flag, one meaning

**Pitfall**: merging "how deep to scan" and "how many rows to return" into one flag; or letting a "compact mode" print *more* than the default in some cases.
**Consequence**: callers cannot control probe depth and response size independently; tokens run away.
**Rule**: scan depth (`--limit`) and returned rows (`--top`) are orthogonal — separate flags. Compact flags (`--compact`/`--summary`) may only ever shrink output; `--fields` only trims further. Never stuff two meanings into one parameter "to save trouble".

## Invalid flags must never be accepted-then-ignored

**Pitfall**: many subcommands share one big flag set; a subcommand parses a parameter its execution path never reads. Variant: the CLI forwards the flag to the server, but the server silently ignores it (e.g. a readonly form field) while the CLI still reports success.
**Consequence**: the agent sees exit 0 and believes the filter or time window applied, while the evidence doesn't match the request — more dangerous than a loud error.
**Rule**: each subcommand registers only parameters that actually take effect; `describe` and the parser share one definition. Retired parameters return usage errors with a replacement command; when a compatibility window is needed, return a stable warning — never a silent no-op. **Server-side ignoring** can't be blocked at submit time, but then the CLI must carry a warning in the success output (a `warnings` field) pointing at the correct path (e.g. "domain changes need app-level config") — never let the agent believe it took effect.

## Zero hits must state actual coverage

**Pitfall**: a search returns only `count=0`, or writes the post-filter row count as `scanned=0`, saying nothing about raw volume read, time range, pagination/tail truncation, or failed shards.
**Consequence**: the agent summarizes "didn't read", "read only a tail sample", and "read but found nothing" all as "the full range is clean".
**Rule**: return raw reads, post-filter hits, and post-budget output counts separately, plus the actual covered range, cap hits, failure counts, and a `potentially_limited`-style state. Zero-hit conclusions hold only inside proven coverage.

## Page-size parameters for paginated scraping must be verified by experiment

**Pitfall**: a server-rendered platform lists 10 rows per page by default, and the page-size parameter name is not what you'd guess — different pages on the same platform behave differently (one list honors `PageSize` while another silently ignores `Count` and still truncates at 10, with only `PageSize` taking effect). Another variant: scraping only the first page without parsing the pagination controls.
**Consequence**: the CLI exits 0 with plausible-looking data, and the agent concludes "no such task configured" or "no failure records" — audit-grade wrong conclusions; landing on exactly 10/20 rows triggers no alarm. (Real history: the same platform hit both variants within two weeks, both causing audit misjudgments.)
**Rule**: before wiring any paginated list, run the experiment: against a target known to exceed the default page size, compare "default vs candidate page-size parameter vs following pagination links page by page" and count rows each way; parsing pagination controls (e.g. `handler=Jump` links) is the backstop. CLIs pull the full set by default; "exactly the default page size" is a truncation signal — acceptance uses an over-page-size fixture. Companion skills carry the pointer: "when tool conclusions disagree with what the user sees on the page, check the CLI version before re-collecting evidence".

## Repeated serial loops mean the CLI lacks batch primitives

**Pitfall**: the skill makes the agent run the same command per target and per keyword, forming N×M calls.
**Consequence**: latency and tokens scale linearly, mid-run failures are hard to summarize, and different targets may get inconsistent parameters.
**Rule**: sink stable set discovery, OR filtering, bounded concurrency, and per-target summarization into CLI batch/aggregate capability; the skill only picks the scope and explains the result. Batch responses carry per-target terminal states plus overall partial/failures.

## Upstream business success is not HTTP status

**Pitfall**: upstream business failures still return HTTP 200 (success hides inside a business-code wrapper); one business error code means "not logged in / forbidden / not found" across endpoints; sometimes an outer HTTP 500 wraps an inner permission 403. Form-style server-rendered platforms invert it: a 302 redirect *is* submit success, while 200 usually means validation failed and re-rendered.
**Consequence**: a CLI judging by HTTP status calls business failures success (or vice versa), and the agent keeps orchestrating on a false conclusion.
**Rule**: before wiring any upstream, determine the **business success criterion** by experiment (wrapper fields, business-code enums, SSR 302/200 semantics) and converge it into one client-side judgment layer — never scattered per command. When one error code carries several meanings, disambiguate with error-prose keywords and note in a code comment "depends on server wording; sync the word list when wording changes". Gateway/proxy interceptions (non-business responses like HTML 413/502) get their own category — never swallowed as "failed to parse response".

## Write success is judged by re-reading terminal state, not the submit response

**Pitfall**: treating a form/API submit acknowledgment (e.g. a 302 back to the list page) as success without re-reading.
**Consequence**: missing targets, disallowed state transitions, or platform-side rejections get reported as success, and the agent builds follow-ups on a lie.
**Rule**: stateful writes re-query the target after submitting to confirm terminal state: stopped → really stopped; deleted → really gone from the list; triggered → target still present counts as accepted. Missing targets return not_found (exit 3) directly — never fake success. The submit response only proves "submitted"; it pairs with the acceptance-stage "done means converged" rule.

**The reverse holds too**: an HTTP 5xx from the server does not equal a failed action. Real incident: a platform returned 500 for Start while the task had actually started; the CLI judged failure, so the agent left an effective action alone to retry it or hand-drove around it. On server anomalies, don't judge failure directly — run the same terminal-state recheck: recheck passes → judge success with the HTTP anomaly recorded in warnings; recheck fails → judge failure with the anomaly folded into the validation error (kept distinct from ordinary business-validation failures).

## Re-login once and only on session expiry; never retry permission failures or uncertain outcomes

**Pitfall**: auto re-login and replay on any 403 or any failure; auto-replay write requests after network errors/timeouts; persisting the session because the login endpoint returned success.
**Consequence**: 403 is a permission verdict — re-login is useless and hides the real problem; replaying uncertain outcomes double-submits (duplicate tickets, duplicate publishes); "login succeeded but session doesn't work" fake credentials get persisted and every later request fails.
**Rule**: only explicit **session-expiry signals** (401, 302 to the login page, login-form HTML in the response) trigger auto re-login, **at most once**; 403/permission-class errors propagate as-is, never masked by re-login. Write requests with uncertain outcomes (network errors, timeouts) are **never auto-replayed** — the caller decides. After a successful login, verify the session against a read-only endpoint before persisting — an HTTP login success does not equal a working session.

## Credentials must be bound to the login-time service origin

**Pitfall**: the CLI accepts a `--base-url`-style override but sends saved passwords/tokens to the new address indiscriminately; or it allows plaintext HTTP to remote addresses.
**Consequence**: one address parameter ships locally saved passwords to an arbitrary host (credential exfiltration); plaintext HTTP walks credentials down the wire naked.
**Rule**: persist the **normalized origin** (scheme + host + port) observed at login alongside the credential; later requests send it only to that same origin. Commands aimed at another origin refuse to reuse the credential and demand a fresh login — saved passwords/tokens never go to a new address. Remote services require HTTPS; HTTP is allowed only for loopback test addresses.

## Streaming commands are their own event protocol and must carry a budget

**Pitfall**: tail/follow/watch commands emit a one-shot JSON envelope or an unbounded stream; startup failures print one error line and exit; multiple streams each keep their own budget counter.
**Consequence**: agents cannot consume line by line; unbounded streams blow up context and disk; mid-stream failures change the output format and break parsers; N streams multiply the budget N times.
**Rule**: streaming output uses a **JSONL event protocol** (`start` / record / `warning` / `end` event types) with independently parseable lines carrying source context (which target, which stream); **pre-start failures also emit an `end` event carrying the error** — never switch to a text error or an envelope mid-way. Non-terminal mode enables a **bounded budget by default** (time/rows/bytes triple cap, e.g. 60s / 200 rows / 256 KiB); only an explicit `--unbounded` lifts it. Budget expiry is a **normal ending** with a cutoff-reason field — kept distinct from failure and external cancellation. Concurrent streams share **one unified budget counter**, never per-stream accounting.

## Parse failures must point at upgrades, not invite blind retries

**Pitfall**: a CLI scraping HTML/SSR pages or strongly structured responses hits a platform redesign and its parse-failure error only says "parse failed".
**Consequence**: the agent reads an adapter failure as a usage error and retries with different parameters, spiraling further off course.
**Rule**: parse-failure errors on structured-scrape commands must attribute explicitly to "upstream structure changed, this version needs an adapter update", with the hint giving the upgrade/reinstall command; `version --json` and `--help` carry the platform-adapter reminder in parallel. Companion skills carry the pointer: "when tool conclusions disagree with what the user sees on the page, check the CLI version before re-collecting evidence".

## Keep ID and numeric types faithful

**Pitfall**: an upstream ID arrives as a JSON number and the command layer stringifies it while assembling parameters through a generic map.
**Consequence**: the server's parameter validation returns 400 with prose that never suggests a type problem; debugging walks the long way around.
**Rule**: pass upstream fields through with their original JSON types (numbers stay numbers, strings stay strings); type conversion happens only in an explicit presentation layer. When storage and presentation disagree by convention (e.g. a uid stored as a string but displayed as a number), note it in a model comment — don't convert early in the storage layer.

## Follow-up commands must not lose binary identity

**Pitfall**: the skill pinned an absolute path and version, but the CLI's `next_commands` returns a bare command name.
**Consequence**: the agent's next step may hit a stale copy on PATH, and capabilities/flags disagree within one session.
**Rule**: prefer structured argv; when returning executable text, keep the verified absolute binary path. Skills likewise rewrite the entry point to the session-pinned path before running CLI-suggested commands.

## Daemon CLI: when behavior contradicts code, kill the process first, then read code

**Pitfall**: CLIs with a background daemon / shared session (browser control, long-lived proxies) stack two traps: ① several binary copies (dev build vs installed release) point at the same daemon by default, so edited code still yields old behavior; ② env-style config is read once at daemon start, so changed env vars never take effect on later invocations.
**Consequence**: chasing behavior-vs-source discrepancies through code reads gets messier the longer it goes (real incident: two versions sharing one daemon kept emitting old-code output, nearly misdirecting a debugging session; env vars looked "not passed" when the process simply never restarted).
**Rule**:
1. When output contradicts code, first kill and restart the daemon (`kill`/`kill --all`-style commands) and confirm the process actually changed — then suspect code. Reversing this order costs hours.
2. Dev binaries must use a unique session name (`--session dev-<task>`); never share the default session with the installed release.
3. When a setting (timeouts, connection counts) takes effect only at daemon start, its docs/`describe` must say "changing this requires a process restart" — "configurable via env var" alone is forbidden.

## Human text errors and machine JSON must not share one path

**Pitfall**: folding human-facing text errors and machine-facing JSON-envelope errors into one parsing/output path.
**Consequence**: one side's caller contract breaks — machine clients get human prose, or humans stare at raw JSON.
**Rule**: two output paths, each independent. Machine errors are JSON envelopes; human errors are readable prose; never let them contaminate each other. New fields/states ship on both paths together.

## Secrets and identity come only from environment variables

**Pitfall**: accepting arbitrary user identity on the command line; or printing tokens into logs, output, error messages, docs.
**Consequence**: identities can be forged; secrets leak into shell history and git.
**Rule**: secrets and owner identity come only from environment variables — never accept arbitrary identity flags. Secrets/tokens stay out of logs, output, docs, and git; diagnostics print only "auth passed/failed", never secret values. Client-side filtering never replaces server-side auth — ownership checks belong server-side. Self-signed tokens use short lifetimes (e.g. 5 minutes).

## Write-guard flags must be registered and pre-checked locally — missing any of the three deadlocks the command

**Pitfall**: a write command declares "requires --version" (read-before-write) but never registers the flag; with the flag the parser rejects it as unknown, without it the command always fails version_required. (Real deadlock: an agent could only hand-POST the platform API to get around it.)
**Consequence**: the command deadlocks completely, and read-before-write protection becomes an unusable feature; agent workarounds skip every CLI safety gate.
**Rule**: read-before-write commands need all three: ① the version flag registered; ② missing values intercepted locally (no network, no login — straight to confirmation_required + a copy-pasteable read command); ③ pre-write re-read of the target with fingerprint comparison. Regressions enumerate all write commands in tests asserting the flag is registered (invoke with `--version x`, assert no unknown-flag error), so new/refactored commands can't drop it again.

## Agent-facing tips belong in structured output, not just human-readable lines

**Pitfall**: putting judgment-affecting tips like "manually triggered records arrive late; trust detail" only in human-readable output lines that JSON mode (the agent's primary mode) never prints.
**Consequence**: the agent never sees the tip and misjudges "record arrived late" as "trigger didn't fire", orchestrating wrongly.
**Rule**: judgment-affecting tips go into the envelope's `data` (`note`/`warnings` fields) or `error.hint`; human-readable lines are only redundant presentation of the same information. `data` fields follow the same "only add, never change or remove" contract.

## Bypass SOPs are CLI-capability-gap signals — pull them back into the command surface

**Pitfall**: project docs accumulate "bypass the CLI with Python+curl straight at the platform API" troubleshooting SOPs (because the CLI can't write request-body schemas).
**Consequence**: agents depend on the bypass for critical actions long-term; the CLI's auth, read-before-write protection, and structured output all get skipped, and docs cement the bypass as standard procedure.
**Rule**: treat any "go around the CLI" SOP or script as a CLI-surface gap list: prefer pulling the capability back into the CLI (e.g. `--req-body-file` with same-file `$ref` dereferencing for direct writes), then update docs and the companion skill to delete the bypass. This mirrors the main document's "never bypass to patch gaps": that one governs agent behavior in the moment, this one governs the CLI maintainer's reverse loop — a long-lived bypass is itself a backlog item.

## Pin read-command field coverage with reflection tests

**Pitfall**: a read command hand-picks fields for its output and drops 8 of them (including the request body); later troubleshooting that validates bodies against it misjudges everything as empty.
**Consequence**: the agent concludes "field empty / not configured" from incomplete output, and can hardly discover the CLI dropped the fields.
**Rule**: extract read-command output assembly into a pure function covered by reflection-consistency tests over every exported upstream-model field (a forgotten new field turns red immediately); the envelope shape itself gets a lock test too. Shrinking output goes through explicit trim channels (`--fields`/`--summary`); default output never drops fields silently.

## Upstream rate limits are not credential failures

**Pitfall**: reading an upstream 429 as token expiry; or hand-firing repeated requests "to reproduce".
**Consequence**: the burst drains the limit bucket completely, further polluting the diagnosis — and can disturb healthy clients.
**Rule**: rate limits and auth failures get different exit codes/error codes and separate handling. For strictly limited upstream endpoints, query through a cached throttle (e.g. 90s default) with daemon and CLI sharing one cache file. Never hand-fire bursts "to reproduce".

## Tests must isolate real user resources

**Pitfall**: unit/integration tests read and write the real user directory and real credential stores (keychains and friends).
**Consequence**: test runs pollute local credentials, pop system authorization dialogs, and damage user config.
**Rule**: every read/write of user directories, credential stores, and backup directories honors one environment-variable redirect (e.g. `TOOL_CONFIG_HOME`, `TOOL_KEYCHAIN_PATH`); tests point them at throwaway temp directories. Test isolation is a design constraint, not an after-the-fact patch.

## When the entry point is a symlink / multi-channel install, verify the artifact

**Pitfall**: code changed, local behavior didn't — reading it as "the change didn't take".
**Consequence**: the run actually used a stale copy — the local entry is a symlink, or several install channels (package manager + manual install) resolved to the old one.
**Rule**: after reinstalling over it, verify the running artifact really is the new build via `command -v <cmd>`, `readlink`, version, and artifact hash (`shasum`). Daemon services also restart with a version check.

## After changing parse logic, spot-check real data

**Pitfall**: testing data parsing/scanning logic only against hand-written toy samples.
**Consequence**: real-world odd values stay uncovered — JSON `null` vs "field missing" differ, some status field is unrelated to the prose content, the field distinguishing humans from system events gets ignored.
**Rule**: after changing scan/parse/preview logic, spot-check ≥5 real records. Hand-written samples prove the happy path only, never robustness against real shapes.

## Read-only boundaries stay sealed

**Pitfall**: gradually adding execute/write commands to a purely read-only data interface until the boundary dissolves.
**Consequence**: the data interface becomes an executor with a blurred responsibility line; callers can no longer assume "calling it changes nothing".
**Rule**: read-only tools never gain side-effecting commands. More visibility (e.g. exposing true process liveness) doesn't break the read-only boundary, but "dispatch instructions" or "start sessions" do. When execution capability is genuinely needed, build a separate tool — don't pollute the read-only interface.

## Write failures must surface in the exit code, never hide in data alone

A deployment CLI's `batch deploy` lesson: failed targets exited 0 while the body printed a raw Go struct (`&{ID:... Message:submit failed: ...}`) — the auditing agent had to parse struct prose by hand to discover the failure, after being blocked twice by unknown flags and walking a full dry-run/preflight/yes-wait round only to find the platform rejection buried deep in struct fields. Iron rules for write-class commands:

- Any failed target → non-zero exit (separate "command didn't run" from "ran but failed"); the failure reason leads the error message, never buried in data struct fields.
- Text mode never `%+v`-dumps structs — render per-target summary lines (status icon + alias + message).
- Known platform-side rejections (e.g. "only certain customers support this") carry next-step guidance in the message; never make the agent guess.

## A borrowed login must stay shared, not forked

**Pitfall**: a CLI reuses the OAuth login another official tool stored (its token file or OS credential store), then refreshes the token on its own schedule and keeps the result privately — or rewrites the file in its own format.
**Consequence**: refresh tokens rotate; the first refresh silently invalidates the other tool's copy, and the user gets logged out of the tool they actually signed into. A rewrite that drops unknown fields breaks that tool's next upgrade.
**Rule**: read the owner's store read-only by default. Refresh only when the token is about to expire or the server answers 401 once; take an exclusive lock, **re-read the store** (another process may already have refreshed), and only then call the token endpoint. Write rotated tokens back atomically into the owner's file in the owner's format, preserving every unrelated field, number, and file mode. Stores you cannot write safely (e.g. an OS keychain item owned by the other tool) stay read-only: report `auth_refresh_failed` with "run the owner tool once" as the next command. Replay only the 401-rejected request, once.

## Quota-spending generation needs a spend ledger to converge

**Pitfall**: a command that spends paid or subscription quota (image/audio/video generation, paid model calls) is "idempotent" only in the sense that it writes the output file again — every rerun buys a new result.
**Consequence**: an agent retrying after an interruption, or re-running a batch to fill gaps, pays again for outputs that already exist; nobody can tell which run produced which file.
**Rule**: fingerprint each request (every input that shapes the output, inputs by content hash) and append intent (`pending`) before the paid call and the outcome (`succeeded` with the output hash, or `failed`) after, in an append-only ledger with actor and timestamp. A rerun whose output file still matches a `succeeded` entry for the same fingerprint returns `unchanged` and spends nothing; an existing output from anything else is `output_exists` (conflict exit) unless `--force`. Store prompts only as part of the fingerprint when they may be private.

## Never auto-retry or auto-switch routes after an uncertain paid call

**Pitfall**: on timeout or an unexpected upstream error, the CLI retries or falls back to a second route (another endpoint, a wrapper around another tool) "to be helpful".
**Consequence**: a timeout does not mean nothing was charged; the fallback buys the same result twice and can hide that the primary route changed.
**Rule**: replay automatically only (a) requests the server provably rejected without effect (a 401 before refresh) and (b) at most once, after a short pause, a connection that dropped before any response byte — proxies and flaky links make this common, and bouncing every hiccup to the user is worse than the small risk of a double charge, which `attempts` makes visible. Timeouts return `timeout` with "quota may already be spent"; a vanished private endpoint returns `route_unavailable` and the fallback route is an explicit flag the caller chooses. Quota exhaustion gets its own exit code and `resets_at`, and a batch stops starting new jobs after a quota or auth failure, marking the rest `not started` so a rerun converges.

## Advisory parameters must be echoed with what actually happened

**Pitfall**: forwarding `size`/`quality`/`model` to an upstream that treats them as hints, then reporting the requested values as if they were the result.
**Consequence**: the agent lays out a page for 1024×1024 and receives 1536×1024; the mismatch surfaces only downstream.
**Rule**: return `requested` (what was sent), `reported` (what the upstream says it did), and measured facts read from the artifact itself (pixel size, bytes, hash). Document which parameters are advisory in `describe` and the flag help.

## Binary artifacts go to files; stdout carries facts

**Pitfall**: printing base64 image/audio data in the JSON envelope, or writing the output in place where a crash leaves a truncated file.
**Consequence**: megabytes of tokens per call; a half-written file that the next run mistakes for a finished result.
**Rule**: require an explicit `--out`, write to a temp file in the target directory and rename, refuse to write through symlinks or non-regular files, and verify the artifact's magic bytes before renaming. The envelope returns the absolute path plus facts (bytes, hash, dimensions, format). Edits that would overwrite one of their own inputs need explicit `--force`.

## Docs and generated skill text carry no machine identifiers

**Pitfall**: a CLI's `AGENTS.md`, README, companion `SKILL.md`, error hints, or generated header lines (e.g. a hook that stamps `<!-- source: /Users/alice/... -->`) contain the author's absolute home path, username, or private network addresses.
**Consequence**: the repository or skill is open-sourced later and leaks personal and infrastructure details; generated lines re-insert them after every manual cleanup.
**Rule**: write repository-relative or `~/`-relative paths and public hostnames or SSH aliases; keep internal addresses in one private infrastructure record and refer to it by name. Fix generators at the source, then clean their output. Install scripts read private endpoints from environment overrides with a public-hostname default.
