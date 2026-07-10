# agent-friendly-cli

> A **Claude Code** / **Codex CLI** skill for designing, building, and reviewing command-line tools that AI agents can call reliably — and that humans can still read and use. It packages a layered design standard (P0/P1/P2), real-world pitfalls, and an evidence-based acceptance checklist into a reusable workflow.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Claude Code](https://img.shields.io/badge/Claude%20Code-Skill-5A67D8?logo=anthropic&logoColor=white)](https://claude.ai/code)
[![Codex CLI](https://img.shields.io/badge/Codex%20CLI-Skill-10A37F?logo=openai&logoColor=white)](https://github.com/openai/codex)

**Languages:** [English](#english) | [中文](#中文)

---

<a id="english"></a>
## English

### Why this exists

Traditional CLIs are built for humans: they use color, ask `[y/N]`, and expect you to guess what an error means. When an AI agent drives that same CLI it can't see color, hangs forever on a confirmation prompt, and burns tokens (or misreads results) on a raw text dump. This skill encodes what it takes to make a CLI **low-token, low-ambiguity, low-risk, auditable, reproducible, and reversible** for agents — *without* making it worse for the humans who still use it.

It's a skill (guidance + reusable workflow), not a library or framework. Reach for it whenever you design, build, or review a CLI — especially one an agent will call.

### What it covers

- **Dual audience by design.** Human-readable by default; machine contract (`--json`) on demand. Auto-detect non-TTY via `isatty()`, split human/machine fields, keep both output paths correct.
- **P0 hard requirements** — the things without which an agent simply can't use the tool: non-interactive mode, a `{ok, data, error, meta}` JSON envelope (failures included), layered exit codes, dry-run, verification commands (`status`/`verify`/`doctor`), and input validation.
- **P1/P2 upgrades** — self-describing `describe`, structured errors with `hint`/`next_commands`, output-size control, write-ahead logging, composability, and CLI/MCP parity.
- **Real pitfalls** — dry-run that secretly writes or spends money, idempotency as an acceptance signal, secrets that leak into logs, rate-limits misread as auth failures, tests that touch real user resources, and more. Each is a `problem → consequence → rule`.
- **Acceptance that isn't self-attested** — a checklist that requires pasting the actual command and its real output as evidence, plus a methodology for testing the CLI with a real agent and reading the raw transcript.

### Quick Install

**Claude Code:**

```bash
git clone https://github.com/x0c/agent-friendly-cli-skill.git ~/.claude/skills/agent-friendly-cli
```

**Codex CLI:**

```bash
git clone https://github.com/x0c/agent-friendly-cli-skill.git ~/.codex/skills/agent-friendly-cli
```

Restart your agent. The entry point is `SKILL.md`; detailed guidance lives in `references/` (design principles, pitfalls, verification) and is loaded on demand. `agents/openai.yaml` provides the Codex-specific interface metadata.

### What's inside

- `SKILL.md` — orchestration and routing: core philosophy, the P0 quick table, and when to read each reference.
- `references/design-principles.md` — the full P0/P1/P2 standard plus the human-and-machine dual-audience rules and contract-evolution discipline.
- `references/pitfalls.md` — battle-tested mistakes to avoid, each as problem → consequence → rule.
- `references/verification.md` — the evidence-based acceptance checklist and the real-agent evaluation methodology.

This skill is language-agnostic: it governs CLI **contract design and acceptance**, and composes with language-specific skills (e.g. a Go CLI skill) that handle implementation details.

### License

[MIT](LICENSE)

---

<a id="中文"></a>
## 中文

### 为什么做这个

传统 CLI 是给人用的：靠颜色、按 `[y/N]` 确认、让你猜报错什么意思。当 AI Agent 去调同一个 CLI，它看不懂颜色、会永久卡在确认框上、拿到一大坨原始文本还会浪费 token 甚至理解错。这个 skill 把「让 CLI 对 Agent 做到**低 token、低歧义、低风险，且可审计、可复现、可回滚**」需要的东西沉淀下来——同时**不牺牲**仍在用它的人的体验。

它是一个 skill（设计规范 + 可复用流程），不是库也不是框架。设计、开发或评审 CLI 时都可以用，尤其是会被 Agent 调用的那种。

### 覆盖什么

- **人机双受众定位**：默认人类可读，按需给机器契约（`--json`）；用 `isatty()` 自动识别非 TTY，人机字段分离，两条输出路径都保持正确。
- **P0 硬性要求**——没有这些 Agent 根本用不了：非交互模式、`{ok, data, error, meta}` JSON 信封（失败也走它）、分层退出码、dry-run、验证命令（`status`/`verify`/`doctor`）、输入校验。
- **P1/P2 提升项**——自描述 `describe`、带 `hint`/`next_commands` 的结构化错误、输出体积控制、写前日志、可组合性、CLI/MCP 一致性。
- **实战踩坑**——dry-run 偷偷写数据或花钱、把幂等当验收信号、密钥泄进日志、限流被误判成鉴权失败、测试碰真实用户资源等，每条都是「坑 → 后果 → 规则」。
- **不靠自陈的验收**——清单要求逐项粘出实际命令和真实输出作证据，另附用真实 Agent 跑多步任务、看原始 transcript 的评测方法。

### 快速安装

**Claude Code：**

```bash
git clone https://github.com/x0c/agent-friendly-cli-skill.git ~/.claude/skills/agent-friendly-cli
```

**Codex CLI：**

```bash
git clone https://github.com/x0c/agent-friendly-cli-skill.git ~/.codex/skills/agent-friendly-cli
```

重启你的 Agent 即可生效。入口是 `SKILL.md`；详细内容在 `references/`（设计规范、踩坑、验收），按需加载。`agents/openai.yaml` 提供 Codex 专用界面元数据。

### 里面有什么

- `SKILL.md`——编排与路由：核心哲学、P0 速览表、每份 reference 的读取时机。
- `references/design-principles.md`——完整的 P0/P1/P2 规范，加人机双受众规则和契约演进纪律。
- `references/pitfalls.md`——实战避坑，每条「坑 → 后果 → 规则」。
- `references/verification.md`——逐项粘证据的验收清单，加真实 Agent 评测方法论。

这个 skill 与语言无关：它管 CLI 的**契约设计与验收**，可以和语言级 skill（比如 Go CLI skill）组合使用，后者负责实现细节。

### 许可证

[MIT](LICENSE)