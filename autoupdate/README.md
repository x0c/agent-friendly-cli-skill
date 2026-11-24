# autoupdate — background self-update for CLIs

`autoupdate` keeps a CLI up to date from **prebuilt binaries committed to a git repository's `dist/` directory** (with a `dist/manifest.json` recorded by the build), replacing the running executable atomically and silently — stdout/stderr are never touched, so `--json` output stays clean.

## How it works

- Each CLI calls `MaybeWorker()` first (the process exits early if it *is* the update worker), then `Start(Options{...})` which spawns a **detached worker subprocess**; the worker does the check and exits.
- The worker maintains a bare shallow clone in the user's cache directory, fetches the target branch, and exports the artifact and manifest from the **same commit** (two independent fetches can never mix versions).
- Update predicate: compare versions first (forward-only), then hashes. If the remote manifest version is not higher than the local `LocalVersion`, the replacement is skipped — a stale `dist/` on the branch can never roll machines back. Set `MC_CLI_AUTO_UPDATE_ALLOW_DOWNGRADE=1` to force pure hash alignment (intentional rollbacks).
- Any artifact whose hash disagrees with the manifest is rejected. A manifest that is **deterministically absent** (git explicitly reports the path missing) degrades to pure hash alignment, recorded as `updated_no_manifest`; network failures and parse failures fail closed — the local binary is never replaced on an uncertain fetch.
- Results — including skip reasons and errors — are written to `<cache>/mc-cli-autoupdate/<command>.json` for observability; successful checks throttle to once per 24h, failures back off for one hour.

## Trust model (read this before adopting)

- The update source is **whatever repository you pass as `Options.GitURL`**. Trust is exactly "anyone who can push to that repository's branch" — there is **no code signing**. If your `dist/` branch is compromised, so is every machine that auto-updates from it.
- The git subprocess runs with a sanitized environment (`GIT_SSH_COMMAND`, git-config overrides, and proxy variables are stripped), so a hostile local environment cannot redirect the clone; the cached bare repo is rebuilt whenever its `origin` stops matching the configured `GitURL`.
- An empty `Options.GitURL` disables the binary update path (recorded as `skipped_no_git_url`). `Options.SkillName` + `Options.SkillRepoMarker` additionally keep a companion agent skill in sync via the open [`npx skills`](https://github.com/vercel-labs/skills) ecosystem — the marker string must appear in the skill-lock source URL, proving the skill came from your skills repository. Leave the marker empty to accept any tracked source.
- Kill switches: `MC_CLI_NO_AUTO_UPDATE=1`, `<COMMAND>_NO_AUTO_UPDATE=1`, or `CI=true`. If the target directory is not writable, updates are skipped.

`MC_CLI_*` is a historical environment-variable prefix; treat it as part of the public contract.
