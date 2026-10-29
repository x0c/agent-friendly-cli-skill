# Distribution and Lifecycle: Background Self-Update, Artifact Discipline, and the Closure Loop

Read this when the CLI's trusted distribution source is a prebuilt-artifact directory inside its repo (e.g. `dist/`). This model fits environments where platform Release features are unavailable or untrusted; CLIs distributed via package managers or Release attachments can adopt the parts that apply (version injection, forward-only updates, and the closure loop hold for any distribution model). Go build specifics live in [go-engineering.md](go-engineering.md).

## 1. Prebuilt artifacts and manifest discipline

- **Artifacts live in the repo, not in platform Releases**: commit every platform's prebuilt binaries straight to the repo's artifact directory; both the install script and self-update pull from that directory, keeping "the official repo = the only trusted source".
- **The manifest is a machine-checkable contract**: after building all platform artifacts, the build script generates a manifest file (e.g. `dist/manifest.json`) recording per-artifact sha256, the version, and the build commit. The self-update version guard and install verification both depend on it — **commit it in the same batch as the artifacts**.
- **Source changes require an artifact rebuild**: pushing source without rebuilding leaves the repo in "code new, artifacts stale" state — self-update then **aligns every machine back to the old artifacts**, silently rolling users back, and the damage surfaces only days later (real incident shape). Block it mechanically before push, not by discipline alone: a check script asserts "no source changes since the manifest's build commit + the artifact set covers every platform + every hash matches the manifest", and a pre-push hook runs the same script. When the manifest is missing or either side's version is unparseable, fall back to hash alignment with an explicit warning.
- **`.gitignore` must not ignore the artifact directory**: the artifacts are deliverables, not build junk.

## 2. The minimal complete background self-update spec

These eight rules are the **self-contained spec** — they do not depend on any external repo documentation:

1. **Separate subprocess**: the update runs in a dedicated child process (the entry point first checks "am I the update worker?" — if so, do the work and exit; otherwise spawn the worker asynchronously and run the main command normally). Never run updates on a bare goroutine inside the CLI process — short commands exit and kill the update mid-flight, and half-downloaded temp files pollute the cache. Also **never wire the update entry into the test-only command path**; unit tests only exercise paths that skip update startup.
2. **Silent**: write nothing to stdout/stderr by default (avoid polluting `--json` output); state and results go to a dedicated status file under the user cache directory for later inspection.
3. **Update predicate: compare version first, then hash — forward-only**: fetch the remote manifest and compare versions first — when the remote version is not higher than local (typical: remote artifacts stale, not rebuilt), **skip the replacement**, preventing a newer local build from being pulled back to old artifacts; only when the remote version is higher, download the artifact, compare hashes, and atomically replace on match. Missing manifest or unparseable versions degrade to pure hash alignment. **Never default to pure hash alignment** — it is the direct root cause of the "stale remote artifacts → silent fleet-wide rollback" incident.
4. **Downgrade escape hatch**: for a deliberate artifact rollback, provide an explicit environment variable (e.g. `<TOOL>_AUTO_UPDATE_ALLOW_DOWNGRADE=1`) that restores pure alignment semantics; the default behavior is always forward-only.
5. **Throttling**: after a successful check (`up_to_date`/`updated`), skip re-checking for 24 hours; **failures get only a short backoff (~1 hour)**, never a 24-hour ban — a failure is exactly when a retry is needed soon.
6. **Three kill-switch channels**: a global switch (e.g. `*_NO_AUTO_UPDATE=1`) + a tool-specific switch (e.g. `<TOOL>_NO_AUTO_UPDATE=1`) + automatic disable under `CI=true`; skip silently when the target directory is unwritable.
7. **Version injection and self-check**: `version`/`build_commit` come from real version descriptions injected at build time (e.g. `git describe --tags --always`) and the commit id — **never a hardcoded `v0.1.0`-style default used to decide updates** — the version guard consumes exactly this value. A mistyped injection path **fails silently** (compilation still passes, artifacts carry defaults), so after building, run `version --json` once and verify the injection actually took effect.
8. **Fetching and the trust model**: pull artifacts via a shallow clone of the repo + single-file export, never assuming the host supports partial clone; the git subprocess runs in a sanitized environment (strip caller-injected SSH command overrides, proxies, and custom git config, so the update source cannot be hijacked through the environment). **The trust model must be documented explicitly**: this scheme trusts "whoever can push artifacts to the distribution repo" plus the local machine's credentials; there is no code signing; a hash inconsistent with the manifest refuses the replacement.

## 3. Companion-skill reverse sync (when a companion skill exists)

The install path solves "guide the agent to install when the CLI is missing"; the reverse link needs managing too — when the skill updates, agents on machines that already installed it never find out. Conventions:

- **Fully delegate to the skill manager**: in the same self-update worker, the CLI runs the skill manager's own update command (stdin closed, telemetry disabled, output never touching the terminal) — **never build per-agent-directory sync of your own**.
- **Eligibility reads only the skill manager's global lock**: participate only when the target skill is tracked by it and its source points at the distribution repo; no lock, untracked, or foreign source → skip silently — never act on behalf of another distribution mechanism.
- **Authoritative-machine exemption**: when the machine holds a local authoritative source directory for that skill, skip the sync — prevents unpushed local updates from being overwritten by stale repo content.
- **Mutual exclusion**: several CLIs' workers running skill updates at once contend for the same lock file; serialize through an exclusive lock in the cache directory; stale locks (older than a few minutes) break automatically.
- **Independent state and throttling**: skill sync keeps its own status file (success 24h / failure 1h), recorded separately from the binary self-update so neither blocks the other; disabling follows the same switch system, plus a dedicated `*_NO_SKILL_SYNC`-style switch.
- **Trust model**: equivalent to "whoever can push to the skill distribution repo" + the local skill manager (which executes whatever package code it pulls); skill content directly changes agent behavior, so the trust surface is wider than binary self-update — document it explicitly.

## 4. The post-change closure loop

Never stop at "it compiles" after changing a CLI. Complete the following in order, and in your report honestly separate "paths verified by real runs" from "paths covered by static checks/unit tests only":

1. **Sync every local install entry**: list all entries with `command -v <cmd>` (including the ones first on PATH) and overwrite each with the new artifact — at minimum the package-manager default directory and the user directory. Miss one, and the next session (including other agents on this machine) silently picks up the old version and draws wrong conclusions from old behavior.
2. **Static checks + tests green**: run the project's vet/lint first, then the full test suite.
3. **Rebuild and commit artifacts + manifest**: all-platform artifacts and the manifest file commit in the same batch (see section 1).
4. **Sync docs and the companion skill**: the repo docs, the global skill directory's instructions, examples, and version wording stay consistent with actual behavior, committed together.
5. **Verify the first entry is the new version**: the entry `command -v` resolves to first must already be the new build; for daemon-style CLIs, kill the process first and then verify (see the matching pitfall).
