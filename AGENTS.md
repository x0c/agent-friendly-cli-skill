# AGENTS.md

本仓库是 agent-friendly-cli skill 的源码与发布仓。在本仓库工作时遵守以下约定。

## 仓库结构

- 仓库根即 skill 本体（`SKILL.md` + `references/` + `agents/openai.yaml`）。本地开发权威源在 `~/.config/agentsync/skills/agent-friendly-cli/`，改动先落权威源，再同步到本仓库——不要只改本仓库副本。

## 硬约束

- 发布前必须先 clone 远端、`diff -rq` 比对：远端可能有其他机器推的更新（含仅改远端的修复，如 README 安装 URL），先合并回本地权威源再同步推送，禁止直接 rsync 覆盖。
- commit 身份双字段均为 x0c（`GIT_COMMITTER_NAME`/`GIT_COMMITTER_EMAIL` + `--author`，x0c@users.noreply.github.com），真实姓名邮箱不进提交历史。
- 不推送内网专属内容：公司内部平台的类名、路径、配置不得写入本仓库；示例用通用表述（不写具体公司/平台名）。
- 代码注释、文档默认中文。

## 文档导航

- `README.md`：仓库门面，对外介绍 skill 的适用场景与安装方式；仓库改名或安装方式变化时同步更新其中的 clone URL。
- `SKILL.md`：方法论入口（P0 硬性要求、工作流、契约演进纪律），详细原则按需读 references。
- `references/pitfalls.md`：踩坑清单；`references/design-principles.md`：设计原则；`references/verification.md`：验收清单。
