**Languages:** [English](#english) | [中文](#中文)

# agent-friendly-cli

Design non-interactive JSON command-line tools with exit codes and dry-run for **Claude Code**, **Codex**, and AI agents without hanging on `[y/N]` — still readable for humans.

This is a skill (guidance + review checklists), not a CLI binary and not a framework.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![skills.sh](https://skills.sh/b/x0c/agent-friendly-cli-skill)](https://skills.sh/x0c/agent-friendly-cli-skill)
[![Claude Code](https://img.shields.io/badge/Claude%20Code-Skill-5A67D8?logo=anthropic&logoColor=white)](https://claude.ai/code)
[![Codex CLI](https://img.shields.io/badge/Codex%20CLI-Skill-10A37F?logo=openai&logo=github)](https://github.com/openai/codex)

---

<a id="english"></a>
## English

### Why this exists

Traditional CLIs are built for humans: they use color, ask `[y/N]`, and expect you to guess what an error means. When an AI agent drives that same command line it can't see color, hangs forever on a confirmation prompt, and burns tokens on a raw text dump. Reach for this skill when you design, build, or review a CLI that agents will call.

### The philosophy

1. **One CLI, two audiences.** Machine contract on demand (`--json`, or automatically off-TTY), never at the human's expense; every interactive wizard has a non-interactive equivalent.
2. **Contracts, not prose.** Success and failure share one envelope; exit codes map to real failure modes; errors carry a stable code, a hint, and the literal next commands — the failure tells the agent how to recover.
3. **Safety lives in the CLI, not in the docs.** Models skip written rules. Updates and deletes require reading first (a fingerprint taken at read time is re-checked at write time, which also stops wrong-target operations); high-risk work plans first, and the submit step accepts only that plan.
4. **Done means converged — and proven.** A second run of a write settles to `unchanged`; write success is judged by re-reading terminal state; acceptance pastes actual command output as evidence, and a fresh agent that never saw the design discussion must be able to operate the tool.

### What it covers

- **P0 hard requirements** — non-interactive mode, a `{ok, data, error, meta}` JSON envelope (failures included), layered exit codes, dry-run, verification commands (`status`/`verify`/`doctor`), input validation, and self-serve installation with a trusted download source.
- **P1/P2 upgrades** — self-describing `describe`, structured errors with `hint`/`next_commands` and machine-readable `context`, output-size control, write-ahead logging, composability, CLI/MCP parity.
- **Read-before-write guard** — `--version` optimistic-lock fingerprints instead of `--yes`, with create-time duplicate checks and post-write token echo.
- **Real pitfalls** — 38 battle-tested entries, each a `problem → consequence → rule`: dry-run that secretly writes or spends money, upstream business codes that hide inside HTTP 200, pagination traps, rate limits misread as auth failures, tests that touch real user resources, and more.
- **Distribution and lifecycle** — background self-update (forward-only version guard, throttling, kill switches), prebuilt-artifact and manifest discipline, version injection, and the post-change closure loop that syncs every local install path.
- **Acceptance that isn't self-attested** — an evidence-per-item checklist, plus a methodology for testing the CLI with a real agent, reading the raw transcript, and auditing production session histories by recovery steps.

### Quick Install

Requires Node.js (for the `skills` CLI):

```bash
npx skills add x0c/agent-friendly-cli-skill -g
```

This installs to every detected agent host (Claude Code, Codex CLI, Cursor, …). Restart your agent. The entry point is `SKILL.md`; detailed guidance lives in `references/` and is loaded on demand. `agents/openai.yaml` provides the Codex-specific interface metadata.

### What's inside

- `SKILL.md` — routing: scope, one-line goal, and which reference to read when.
- `references/design-principles.md` — the P0/P1/P2 standard, dual-audience rules, read-before-write fingerprint recipe, contract-evolution discipline.
- `references/implementation-workflow.md` — the full workflow: P0 checklist, self-serve installation, ask-once credentials, transaction skeleton, high-risk writes, acceptance.
- `references/pitfalls.md` — battle-tested mistakes to avoid, each as problem → consequence → rule.
- `references/distribution.md` — background self-update, prebuilt artifacts and manifest discipline, post-change closure loop, companion-skill reverse sync.
- `references/go-engineering.md` — Go engineering: cross-compilation matrix, syscall platform split, ldflags version injection, shared-package reuse rules.
- `references/verification.md` — the evidence-based acceptance checklist and real-agent evaluation methodology.

This skill is language-agnostic in its main line: it governs CLI **contract design and acceptance**, and composes with language-specific references (Go engineering included here).

### Related Tools in the AI Coding Suite

If you run multiple AI coding agents daily, check out the rest of the local-first toolkit:

| Repository | Description |
| :--- | :--- |
| **[corral](https://github.com/corral-dev/corral)** | Run Claude Code, Codex, OpenCode, Cursor & Pi side-by-side in a terminal session manager, then continue from iPhone or Mac. |
| **[subswap](https://github.com/x0c/subswap)** | Check remaining quota and switch Claude Code, Codex, Cursor, Kimi Code & OpenCode accounts without logging out. |
| **[agentsync](https://github.com/x0c/agentsync)** | Sync `AGENTS.md`, `CLAUDE.md`, skills, and MCP configs across coding agents from one local source of truth. |
| **[agent-doc-skills](https://github.com/x0c/agent-doc-skills)** | Navigable project knowledge base skills (`doc-init`, `doc-update`, `doc-compact`) so coding agents don't rediscover your repo every session. |

### License

[MIT](LICENSE)

---

<a id="中文"></a>
## 中文

### 为什么做这个

传统 CLI 是给人用的：靠颜色、按 `[y/N]` 确认、让你猜报错什么意思。当 AI Agent 去调同一条命令行，它看不懂颜色、会永久卡在确认框上、拿到一大坨原始文本还会浪费 token。设计、改造或评审「Agent 也要调、人也能用」的 CLI 时用这个 skill。它是规范 + 验收清单，不是一条可安装的命令，也不是框架。

### 设计哲学

1. **一个 CLI，两个受众**：机器契约按需出现（`--json` 或非 TTY 自动切换），不以牺牲人类体验为代价；每个交互向导都有非交互等价路径。
2. **契约优先于文字**：成败共用一个信封；退出码映射真实失败模式；错误带稳定短码、hint 和可照抄的后续命令——报错本身就告诉 Agent 怎么恢复。
3. **安全长在 CLI 里，不长在文档里**：模型会跳过文字规则。改/删必须先读后写（读时取指纹、写时重验，顺带挡住操作错对象）；高风险动作先出不可变计划，提交只认计划。
4. **完成=收敛且可证明**：写命令第二遍收敛为 `unchanged`；写成功以回查终态为准；验收逐项粘真实输出作证据，且必须由没参与设计的全新 agent 独立操作通过。

### 覆盖什么

- **P0 硬性要求**——非交互模式、`{ok, data, error, meta}` JSON 信封（失败也走它）、分层退出码、dry-run、验证命令（`status`/`verify`/`doctor`）、输入校验、带可信下载源的自助安装。
- **P1/P2 提升项**——自描述 `describe`、带 `hint`/`next_commands` 与机器可读 `context` 的结构化错误、输出体积控制、写前日志、可组合性、CLI/MCP 一致性。
- **读后写守卫**——用 `--version` 指纹乐观锁替代 `--yes`，create 走查重，写后回显新令牌。
- **实战踩坑**——38 条「坑 → 后果 → 规则」：dry-run 偷偷写数据或花钱、业务成败藏在 HTTP 200 里、分页陷阱、限流被误判成鉴权失败、测试碰真实用户资源等。
- **分发与生命周期**——后台自更新（只进不退版本守卫、节流、禁用开关）、预编译产物与清单纪律、版本注入、改完同步全部本机安装路径的收尾闭环。
- **不靠自陈的验收**——逐项粘证据的清单、真实 Agent 多步实测与 transcript 分析、上线后按恢复步数做会话审计。

### 快速安装

需要 Node.js（装 `skills` CLI 用）：

```bash
npx skills add x0c/agent-friendly-cli-skill -g
```

一次装到所有已识别的助手宿主（Claude Code、Codex CLI、Cursor ……）。重启你的 Agent 即可生效。入口是 `SKILL.md`；详细内容在 `references/`，按需加载。`agents/openai.yaml` 提供 Codex 专用界面元数据。

### 里面有什么

- `SKILL.md`——路由：定位、一句话目标、什么场景读哪份 reference。
- `references/design-principles.md`——P0/P1/P2 规范、人机双受众规则、读后写指纹配方、契约演进纪律。
- `references/implementation-workflow.md`——完整工作流：P0 清单、自助安装、凭证「索取一次」、事务骨架、高风险写操作、验收。
- `references/pitfalls.md`——实战避坑，每条「坑 → 后果 → 规则」。
- `references/distribution.md`——后台自更新最小完整规范、预编译产物与清单纪律、收尾闭环、配套 skill 反向同步。
- `references/go-engineering.md`——Go 工程落地：交叉编译矩阵、syscall 平台拆分、ldflags 版本注入、公共包复用红线。
- `references/verification.md`——逐项粘证据的验收清单 + 真实 Agent 评测方法论。

这个 skill 主线与语言无关：管 CLI 的**契约设计与验收**，并与语言级参考（此处含 Go 工程落地）组合使用。

### AI 编程工具链里的相关工具

如果你每天在多个 AI 编程助手之间切换，看看这套本地优先工具链的其余部分：

| 仓库 | 说明 |
| :--- | :--- |
| **[corral](https://github.com/corral-dev/corral)** | 在终端会话管理器里并排跑 Claude Code、Codex、OpenCode、Cursor 和 Pi，再从 iPhone 或 Mac 上继续。 |
| **[subswap](https://github.com/x0c/subswap)** | 查剩余额度，一键切换 Claude Code、Codex、Cursor、Kimi Code 和 OpenCode 的账号，不用反复登录。 |
| **[agentsync](https://github.com/x0c/agentsync)** | 以一份本地源统一同步 `AGENTS.md`、`CLAUDE.md`、skills 和 MCP 配置到各编程助手。 |
| **[agent-doc-skills](https://github.com/x0c/agent-doc-skills)** | 可导航的项目知识库 skills（`doc-init`、`doc-update`、`doc-compact`），让 coding agent 不再每个会话都重新认识你的仓库。 |

### 许可证

[MIT](LICENSE)
