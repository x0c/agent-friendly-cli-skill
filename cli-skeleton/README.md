# cli-skeleton: a starter template for new CLIs

Every new CLI starts by copying this directory — never by copying boilerplate
from older projects. After copying:

1. Globally replace `mcskel` / `cli-skeleton` / `MCSKEL_CONFIG_HOME` / `MCSKEL_PASSWORD` with your command and directory names.
2. `internal/cmd/root.go`: set the command name, platform copy, capabilities list, and subcommand registration.
3. `internal/cmd/login.go`: wire the real platform login endpoint and success check as described in the file header comment.
4. `internal/cmd/profile.go`: add platform-specific profile fields and validation.
5. `Makefile`: update COMMAND and VERSION_PKG, then still include the top-level `../build.mk`.
6. When officially releasing (artifacts go into the repository's `dist/`), enable the self-update injection as described in the comments in `root.go`, and add the require + replace directives for `github.com/x0c/agent-friendly-cli-skill/autoupdate` to go.mod.

## Shared capabilities already wired in this template

| Shared package | Purpose | Where it appears in the template |
|---|---|---|
| `appframe` | Entry-point wiring: error→exit-code mapping, dual envelope/human output | `Execute` in root.go |
| `versioncmd` | Standard version command (ldflags triplet + capabilities) | root.go |
| `profile` + `prompt` | Multi-profile credential storage (0700/0600/atomic writes) and password reading | profile.go |
| `writeguard` + `fingerprint` | Read-then-write optimistic locking: missing version → `confirmation_required` (exit code 2), mismatch → `conflict` (exit code 5) | the update command in profile.go |
| `rest` | HTTP request skeleton: mandatory timeouts, bounded reads, typed errors | login.go / doctor.go |
| `session` | Cookie session restore + `Ensure` auto re-login orchestration | login.go |

## This template is not published

- It is not part of the repository's dist release artifacts, and self-update stays disabled (see the comments in root.go).
- It is compiled along with the repository to keep the template from drifting away from the shared-package APIs: `cd cli-skeleton && go build ./...`.

## Smoke self-check

```sh
cd cli-skeleton
go build -o /tmp/mcskel .
MCSKEL_CONFIG_HOME=/tmp/mcskel-t1 /tmp/mcskel profile add --name t --server https://example.com --username u
MCSKEL_CONFIG_HOME=/tmp/mcskel-t1 /tmp/mcskel profile show --name t      # grab the version
MCSKEL_CONFIG_HOME=/tmp/mcskel-t1 /tmp/mcskel profile update --name t --username u2 --version <version-from-above>
MCSKEL_CONFIG_HOME=/tmp/mcskel-t1 /tmp/mcskel profile update --name t --username u3   # without --version → confirmation_required
```
