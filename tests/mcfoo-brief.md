# Task brief: build the internal-platform CLI "mcfoo"

You are a CLI development agent. Using only the skill at `<absolute skill directory path>` (SKILL.md and every file it references), produce a development plan ready to hand to an implementer.

**Constraints**: you may read only files inside that skill directory; do not read, search, or assume the existence of any other internal repository, internal standards file, or machine-local global config. For every decision beyond the skill, mark it "skill silent → my own call".

## Requirements

Build the internal-platform CLI **mcfoo** (Go):

- `login` (username/password)
- `widget list / get / create / update` subcommands
- `--json` output for stable AI-agent use; writes must be guarded against agent mistakes
- distributed to colleagues, with automatic updates on their machines
- a release and test process to go with it

## The plan must cover (each item annotated with its basis: which skill file and section, or "skill silent → my own call")

1. Command surface and parameter design
2. The exact output JSON envelope fields and failure-path behavior
3. The exit-code table
4. The write guard (where `version` comes from, what the algorithm is, what the rejection error looks like)
5. Credential storage (file format, permissions, expiry re-login, first-login guidance)
6. The self-update mechanism (process model, update source, version and hash verification, throttling, how to disable)
7. The build-and-release process (platform matrix, artifact manifest, how source/artifact sync is enforced, where the version number comes from)
8. The post-change closure checklist
9. The test and acceptance checklist

After the plan, add one separate section: "points the skill did not cover, so I decided on my own".
