# cli-skeleton: starter template for new CLIs

Every new CLI starts by copying this directory — do not copy boilerplate from an old project. After copying:

1. Globally replace `mcskel` / `cli-skeleton` / `MCSKEL_CONFIG_HOME` / `MCSKEL_PASSWORD` with your command and directory names.
2. `internal/cmd/root.go`: change the command name, platform copy, capabilities list, and subcommand registration.
3. `internal/cmd/login.go`: wire the real platform login endpoint and success predicate per the header comment in the file.
4. `internal/cmd/profile.go`: add platform-specific Profile fields and validation.
5. `Makefile`: after changing COMMAND and VERSION_PKG, keep including the root-level `../build.mk`.
6. For the official release (artifacts into the repo's `dist/`), enable the autoupdate wiring per the comments in `root.go`, and add the require + replace for `github.com/x0c/agent-friendly-cli-skill/autoupdate` in go.mod.

## Public packages already wired into the template

| Package | Purpose | Location in the template |
|---|---|---|
| `appframe` | Entry assembly: error→exit-code mapping, envelope/human dual-mode output | Execute in root.go |
| `versioncmd` | Standard version command (ldflags triple + capabilities) | root.go |
| `profile` + `prompt` | Multi-profile credential storage (0700/0600/atomic writes) and password reading | profile.go |
| `writeguard` + `fingerprint` | Read-before-write optimistic lock: missing version→confirmation_required (exit code 2), mismatch→conflict (exit code 5) | update command in profile.go |
| `rest` | HTTP request skeleton: enforced timeouts, read limits, typed errors | login.go / doctor.go |
| `session` | Cookie session restore + Ensure auto-relogin orchestration | login.go |

## What this template does not ship

- Not in the repo's dist release manifest, no autoupdate enabled (see the comments in root.go).
- Compiled and tested together with the repo to prevent drift between the template and public package APIs: `cd cli-skeleton && go build ./... && go test ./...` (real-world lesson, 2026-11-24: building without testing once missed a stale assertion — when the appframe contract evolved to unknown_command/exit code 2, the template's tests had not kept up).

## Smoke self-check

```sh
cd cli-skeleton
go build -o /tmp/mcskel .
MCSKEL_CONFIG_HOME=/tmp/mcskel-t1 /tmp/mcskel profile add --name t --server https://example.com --username u
MCSKEL_CONFIG_HOME=/tmp/mcskel-t1 /tmp/mcskel profile show --name t      # grab the version
MCSKEL_CONFIG_HOME=/tmp/mcskel-t1 /tmp/mcskel profile update --name t --username u2 --version <version from above>
MCSKEL_CONFIG_HOME=/tmp/mcskel-t1 /tmp/mcskel profile update --name t --username u3   # without --version → confirmation_required
```
