---
name: agent-friendly-cli
description: 新建、改造或评审命令行工具（CLI）时使用，尤其是供 AI Agent 稳定调用、同时也要给人正常使用的 CLI。覆盖命令面设计、非交互模式、JSON 输出契约、退出码分层、dry-run、幂等、鉴权安全和验收方法。
---

# 面向 Agent 的 CLI 工具开发

## 定位

这个 skill 管**跨语言的 CLI 契约设计与验收**：命令怎么分、输出怎么给、退出码怎么排、非交互怎么处理、怎么避坑、怎么验收。语言层面的实现细节（如 Go 的 cobra/flag、Python 的 argparse）交给对应语言级 skill，本 skill 不重复。

核心目标一句话：让 CLI 对 Agent **低 token、低歧义、低风险，且可审计、可复现、可回滚**；对人**默认可读、可交互**。这是同一个 CLI 的两个受众，不是两套工具。

## 什么时候读哪个 reference

- 设计命令面 / 输出契约 / 退出码，或评审一个 CLI 是否 agent-friendly → 读 [references/design-principles.md](references/design-principles.md)（P0/P1/P2 分层 + 人机双受众规范）。评审时逐条核对，缺失项就是要报告的问题。
- 动手实现，想避开真实事故 → 读 [references/pitfalls.md](references/pitfalls.md)（dry-run 真只读、幂等信号、密钥安全、测试隔离等踩坑教训）。
- 写完要验收 → 读 [references/verification.md](references/verification.md)（逐项粘证据的验收清单 + Agent 实测评测方法论）。

## 工作流

### 设计阶段

先过 **P0 六条硬性要求**——没有这些 Agent 根本用不了，细节见 design-principles.md：

| P0 | 一句话 |
|---|---|
| 非交互模式 | 检测到非 TTY 自动关交互（用 `isatty()`，stdin/stdout 分开测），不要求调用方主动传 flag |
| 结构化输出 | `--json` 统一 envelope `{ok, data, error, meta}`；失败也走 stdout JSON + 非零退出码 |
| 退出码分层 | 0 成功 / 1 一般失败 / 2 用法错误 / 3 不存在 / 4 权限鉴权 / 5 冲突 / 6 超时 |
| dry-run | 有副作用的命令能预演，输出结构与真实执行一致，且真只读、不花钱 |
| 验证命令 | 提供 `status`/`verify`/`doctor`，让调用方在退出码之外再查一遍 |
| 输入校验 | 硬拦路径逃逸、命令注入 |

过完 P0 再按需要加 P1（`describe` 自描述、结构化错误带 `hint`/`next_commands`、体积控制、写前日志、自动生成 SKILL.md、可组合性）。默认采用**声明式命令**（`ensure`/`apply`）而非命令式（`create`/`delete`），天然幂等安全。

**人机双受众**贯穿始终：默认输出人类可读（表格、颜色可用），`--json` 或非 TTY 时全部剥离只留机器契约；交互向导必须有非交互等价路径（`--yes` + 全量 flag）；同一信息给双字段（程序用英文枚举 `status`，人看本地化 `status_tag`）。

### 实现阶段

对照 pitfalls.md 逐条避坑，重点：dry-run 分支拦住**所有**副作用；写命令做到第二遍收敛为 `ok`；密钥只从环境变量读、绝不进日志/输出/Git；测试全程重定向到临时目录、不碰真实用户资源。

### 验收阶段

按 verification.md 清单逐项**粘实际命令和真实输出**作证据，不接受「已确认」自陈。契约合规（清单）和 Agent 顺畅可用（真实 Agent 多步任务实测 + 看 transcript）都要做。

## 契约演进纪律

JSON envelope、退出码、已发布字段名是对外契约，发布后**只加不改不删**；破坏性变更升接口版本号并显著标注。
