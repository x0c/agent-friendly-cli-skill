# Go CLI Engineering: Cross-Compilation, Version Injection, and Shared Packages

The main-line specs (output contracts, self-update, closure) are language-agnostic — see [design-principles.md](design-principles.md) and [distribution.md](distribution.md); this file covers Go-language and engineering-structure details.

## Cross-compilation platform matrix

- **Six platforms by default**: `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`, `windows/arm64`; name artifacts `<cmd>-<os>-<arch>[.exe]` and output them all under `dist/`. Cross-compile with `GOOS`/`GOARCH`, adding `-trimpath` and `-ldflags "-s -w"`.
- **Collapse build commands into the Makefile**: one `make build-all` target produces every platform's artifacts and generates `dist/manifest.json` at the end (per-artifact sha256 + version + build commit); multiple CLIs share one includable build fragment instead of hand-copying a drifting copy per project.
- **Manifest and push gate**: see distribution.md section 1; manifest generation and the check script share one source, and the pre-push hook runs the same script as the manual command.

## Unix-only syscalls must split per platform

Unix-only calls (`syscall.Setpgid`, `syscall.Kill`, …) must be split into `_unix.go` / `_windows.go` files carrying `//go:build` tags (e.g. `//go:build !windows` vs `//go:build windows`); process-detach/signal-control code — exactly the hot zone for self-update child spawning — deserves extra care. Developing and testing on macOS/Linux **never exercises this failure**; only the Windows cross-compilation after push fails and breaks the whole artifact build. Split pattern: each platform file provides the same function name, and the main logic calls the abstraction only.

## Version injection (ldflags)

```make
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildCommit=$(COMMIT)
```

- Keep only a `var version = "dev"`-style default in source; **never** rely on a hardcoded `v0.1.0` to decide whether an update exists — the version guard consumes exactly this value.
- **A mistyped injection path fails silently**: one extra character in the package path or variable name after `-X` still compiles cleanly while every artifact carries the default. Run `version --json` after building and verify the injection (real incident: two CLIs' Makefile typos left all artifacts on the default version, discovered only from the dist build).
- The main entry wires in the shared self-update package in the order "update-worker check first, then async worker start, then the main command", passing the injected version; never attach it to a test entry.

## Shared-package reuse red lines

- **Extract the shared package when the second CLI appears**: output envelopes, exit-code constants, read-before-write fingerprints, credential storage, password reading, self-update — anything the second CLI would copy must become a shared module (go.mod `require` + `replace` pointing at the local path); new CLIs **must not copy these implementations**. The real cost of tolerated copying: several CLIs each holding their own envelope implementation drift into dialects (context field present/absent, error-code casing, three incompatible meta formats), duplicated code exceeding a tenth of the repo, and every caller site has to move again during the later cleanup.
- **Integrate legacy projects through a thin wrapper**: old projects adopt the shared package via type aliases + function forwarding under `internal/` with zero caller changes; do not "unify" by rewriting every call site.
- **Migrate only what is truly isomorphic**: extraction and consolidation move only parts whose semantics and error pipelines are both isomorphic. Look-alike-but-different (different error carriers, serialization differences, validation living at different layers) does not migrate — forcing it breaks locked contracts. Text and error-code mappings locked by compatibility tests never get "unified" in passing; differences that must stay are preserved via injected parameters.
- **The fingerprint algorithm is a public contract**: the algorithm (canonical-JSON + sha256 truncation) and the serialization live in the shared package; each object declares only its exclusion list and never hand-rolls canonicalization. Switching algorithms invalidates every historical version credential; migrate only with a cross-test against the old algorithm proving identical values (test pattern: run old and new over the same batch of real objects and assert value-by-value equality).

## Engineering structure conventions

- Three layers — `cmd` (argument parsing / output) → `client` (the sole egress to the platform) → `config` (credential storage) — with `output`/shared packages providing the rendering contract horizontally; comments and human-readable output in the product locale, machine-contract key names in English.
- Unit tests use `httptest` stand-ins and temp directories, never real networks or real user directories; credential tests redirect through environment variables into a throwaway config directory.
- The write-path read-before-write guard (version flag registration + local pre-check + pre-write re-read-and-compare) is a shared writeguard module that commands adopt declaratively, so no command can drop the flag and deadlock (see the matching pitfall).

## Bundled Go reference implementation

This repository ships a runnable Go implementation of the contracts this skill specifies, next to the skill body. Consult it before writing your own, and copy the skeleton rather than a mature CLI:

- `cli-common/` (module `github.com/x0c/agent-friendly-cli-skill/cli-common`) — eleven packages: `output` (the `{ok,data,error,meta}` envelope and exit-code constants 0–6), `fingerprint` (canonical-JSON + sha256 version credentials), `writeguard` (declarative read-before-write optimistic locking), `profile` + `secretfile` + `prompt` + `session` (multi-profile credential storage with 0700/0600 discipline, ask-once password reading, cookie-session re-login orchestration), `rest` (forced-timeout, body-limited HTTP skeleton), `tokenrefresher`, `appframe`, `versioncmd`.
- `autoupdate/` (module `github.com/x0c/agent-friendly-cli-skill/autoupdate`) — background silent self-update from prebuilt artifacts committed to a git repo's `dist/`: detached worker subprocess, SHA256 compare, forward-only version guard against `dist/manifest.json`, atomic replace, per-command throttling state, plus companion-skill reverse sync through the open `npx skills` ecosystem. Its trust model is documented in `autoupdate/README.md`.
- `cli-skeleton/` (module `github.com/x0c/agent-friendly-cli-skill/cli-skeleton`) — a compilable starting template wiring the packages above (login, profiles, doctor, version).
- `build.mk` + `scripts/` — six-platform cross-build, `dist/manifest.json` generation, and push-time consistency checks (monorepo layout; pass `--repo-root` for other layouts).

Modules are versioned with `<dir>/vX.Y.Z` tags, so `go get` resolves to a stable version. Runtime messages are centralized in each package's `messages.go`. The `MC_CLI_*` environment-variable prefix and the `mc-cli-autoupdate` cache directory name are historical; treat them as part of the public contract.

These directories are an English snapshot derived from a private production monorepo (translated comments and messages, rewritten module paths, generic examples) and are not synchronized in real time. The skill text is the specification: when text and code disagree, the text wins — file an issue.
