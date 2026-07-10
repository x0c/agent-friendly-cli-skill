# CLI 设计规范：P0 / P1 / P2 分层 + 人机双受众

设计或评审 CLI 的命令面、输出契约、退出码时读本文。评审任务把本文当评判基准：逐条核对，缺失项恰恰是要报告的问题，不要因为被评审代码没用某机制就跳过该条。

## 核心哲学

- 对 Agent：**低 token、低歧义、低风险**，且**可审计、可复现、可回滚**。
- 对人：默认输出可读、可交互，不要求人记住机器契约。
- **渐进式披露**：默认返回精简结果，让调用方按需深挖（`--detail`/`--fields`），不要一次性倾倒全部信息。同一查询从 200+ token 压到 70 token 是常见收益。
- **声明式优于命令式**：能设计成「确保达到某状态」（`ensure`/`apply`/`sync`）就不要设计成「执行一个动作」（`create`/`delete`）。声明式命令在重试、幂等、并发调用下天然安全；命令式命令需要 `--if-not-exists` 或专门的冲突退出码来补救。

## P0：没有这些 Agent 根本用不了

1. **非交互模式**：检测到非 TTY 时自动关闭交互并降级为机器可读输出，而不是要求调用方主动传 `--non-interactive`——Agent 不知道要传这个参数就会卡死在确认框上。实现要点：用 `isatty()` 判断，不要用 `$TERM`；stdin 和 stdout 分开检测（stdout 被管道接走但 stdin 仍是终端的情况真实存在）。
2. **结构化输出**：提供 `--json`（或 `--output json`），统一 envelope：

   ```json
   {"ok": true, "data": {}, "error": null, "meta": {}}
   ```

   失败时 `ok=false`、`data=null`、`error` 含稳定英文短码 `code` 和人可读 `message`；`meta` 放分页、耗时、dry-run 标记等非主体信息。**失败也必须向 stdout 输出合法 envelope + 非零退出码**，不能只在 stderr 打一行文本——错误进结果对象，调用方才能「看到」错误并推理下一步，而不是被当作系统异常吞掉。这一点是**面向 Agent 的刻意取舍**，也和 MCP「错误放在结果对象里而非协议层」的做法一致。

   > **和主流 CLI 的差异要心里有数**：`gh`/`kubectl`/`stripe` 等的 `--json` 输出的是**裸数据**（资源对象本身），不套 `{ok,data,error}` 信封，这样 `jq`/`curl` 管道能直接消费、不用先剥一层 `.data`。信封是**agent-first 的选择**——它给 Agent 一个统一的成功/失败判别位和放 `error.code`/`hint`/`next_commands` 的固定位置，代价是牺牲一点 UNIX 管道直用性。若这个 CLI 的主要消费者是人和 shell 管道而非 Agent，可以考虑输出裸数据、把成败完全交给退出码；主要给 Agent 用时才上信封。两者别混用，选定一种并在文档写明。
3. **stdout / stderr / 退出码严格分离**：结果走 stdout，日志和进度走 stderr。退出码要比「成功/失败」更细粒度，但**按你这个工具真实的失败模式来映射，不是照抄一张固定表**（clig.dev 的原话是「map the non-zero exit codes to the most important failure modes」）。下面是一套合理默认，`0`/`1`/`2` 三档几乎所有工具通用（`2`=用法错误与 argparse、多数 shell 惯例一致），`3` 及以后按需启用：

   | 码 | 语义 |
   |---|---|
   | 0 | 成功 |
   | 1 | 一般失败 |
   | 2 | 用法错误（参数不对） |
   | 3 | 资源不存在 |
   | 4 | 权限 / 鉴权失败 |
   | 5 | 冲突 / 已存在 / 有歧义 |
   | 6 | 超时 |

   **避免用 ≥126 的退出码**——`126`/`127`/`128+n` 被 shell 占用（不可执行 / 命令未找到 / 被信号杀死），自定义码撞上会产生歧义。调用方靠退出码即可分支处理，不必解析文本。文档里明确告知调用方：判断成功看退出码或 `ok` 字段，不要看 stdout 有没有内容。
4. **dry-run**：有副作用的命令必须能先预演。`--dry-run` 只分析不执行，输出结构与真实执行的 `data` 完全一致，方便调用方复用同一套解析逻辑做预演对比。dry-run 的语义约束见 [pitfalls.md](pitfalls.md) 前两条（真只读、不花钱）。
5. **配套验证命令**：调用方不能只信退出码 0 就认为成功，要有 `status`/`verify`/`doctor` 让它再查一遍。操作是声明式的话，验证命令本质上就是重跑一次 `ensure`/`status`，不需要额外机制。`doctor` 检查配置、鉴权、关键下游可达性；`describe` 与 `version` 离线可用（不依赖服务端）。
6. **输入校验**：Agent 可能拼出 `../../etc/passwd` 这种路径，必须硬拦路径逃逸、命令注入。

## P1：有了这些 Agent 成功率显著提升

1. **describe 自描述命令**：`ctl describe <command> --json` 输出机器可读的参数/字段说明，让 Agent 运行时动态查。**describe 的数据必须与参数解析共用同一份定义**（同源），杜绝文档与实际行为漂移。命令本身遵循**名词-动词层级**（`ctl resource verb`，如 `myctl user create`），人和 Agent 都能靠模式识别猜出命令结构，减少查文档次数。
2. **结构化错误 + hint + next_commands**：出错不只给一句话——`error.code`（程序判断）+ `error.hint`（人可读建议）+ `error.next_commands`（调用方可直接照着执行的后续命令）。可加 `error.retryable` 布尔值区分「重试有意义」和「重试无意义」。
3. **体积控制**：
   - `--fields`/`--limit`/`--summary` 让调用方裁剪返回。精简模式（如 `--compact`）只能比默认更小，绝不能更大。
   - **语义标识符优先**：返回值里能用有意义的名字就不要用 UUID/mime_type 这类底层标识，模型对自然语言标识符的处理准确率明显更高。
   - **大结果落盘引用**：单次产出很大（全量导出、大日志）时不要塞进 stdout；写入文件并返回路径 + 摘要（字节数、条数），调用方需要时再读。写文件只允许在调用方显式传 `--out <path>` 时发生，不能变成默认副作用。
4. **operation_id / 写前日志**：每次写操作返回 ID，供后续追查与回滚。更完整的形态是写前日志：操作前记录意图（run_id、idempotency_key、待执行动作），执行后落地结果——中途中断也能判断「这个操作到底有没有发生」。
5. **自动生成 AGENTS.md + SKILL.md**：CLI 自己能教 Agent 怎么用它。SKILL.md（frontmatter `name`+`description` + 正文使用说明）是各主流 coding agent 共同支持的技能发现格式，比要求 Agent 主动跑 `describe` 更省 token。SKILL.md 只写：触发场景、必备环境变量（只列名字不写值）、常用命令、风险说明（哪些命令真实写数据/花钱/发通知）、输出判读（哪些字段表示成功/待处理/失败/可重试）。
6. **可组合性**：`--quiet` 输出裸值方便管道传递；支持从 stdin 读批量输入并明确标记格式（如一行一个 JSON）；避免命令只支持「一次一个」——Agent 经常批处理，逐条调用的 token 开销会迅速累积。

## P2：企业级

MCP 适配、policy-as-code、CI 集成、OpenTelemetry、SDK。若同时提供 CLI 和 MCP 两个接口，底层必须复用同一套业务逻辑和同一套结构化返回格式，只是传输层不同——否则两边行为长期漂移出不一致。

## 人机双受众规范

CLI 同时要让人正常理解和使用，不是纯机器接口：

- **默认人类可读**：不带 `--json` 时输出面向人的格式（表格、颜色、进度条都可以用），带 `--json` 或检测到非 TTY 时全部剥离，只留机器契约。颜色额外遵守通行约定：识别 `NO_COLOR` 环境变量和 `TERM=dumb` 时关闭颜色，并提供 `--no-color` 显式开关。
- **交互 + 非交互双模式**：人用时可以走交互向导（逐项提问、隐藏密钥输入），但同一命令必须提供非交互参数等价路径（`--yes` + 全量 flag）供脚本和 Agent 使用。交互开关的通行命名是 `--no-input`（clig.dev 约定），可与 `--non-interactive`/`--yes` 择一或兼容。
- **人机双字段分离**：同一个信息给两个字段——程序判断用稳定英文枚举（如 `status: "active"`），给人看用本地化文案（如 `status_tag: "🟢 运行中"`）；时间给 `time`（人类可读）+ `mtime`（Unix 时间戳，排序过滤用）。新增状态两边同步，不要让程序去解析带 emoji 的展示字段。
- **错误信息双层**：`error.message` 用面向用户的自然语言（本项目惯例为中文），`error.code` 用稳定英文短码。
- **help 质量**：`--help` 覆盖每个子命令和 flag 的用途与示例；隐藏仅供一次性迁移用的内部命令，不进 help 污染命令面。
- **成功路径输出尽量短**：人和 Agent 都受益；详细 hint 只在失败时出现。

## 契约演进纪律

JSON envelope 结构、退出码分配、已发布字段名是对外契约：**发布过版本后只加不改不删**。破坏性变更必须提升接口版本号（如 `meta.version`）并在文档/SKILL.md 显著标注。已发布字段的取值语义也不能改（例如某字段已发布两个枚举值，不能改成数组）。

## 韧性原则

- 核心操作不依赖非必需的网络查询：状态查询接口坏、token 过期时，主动作（如切换、回滚）仍要能完成。
- 全部依赖不可用时显式失败并保留可审计状态，不要静默降级到危险的默认行为。
- 环境变量做行为开关（如 `TOOL_NO_DAEMON=1`），让用户和 Agent 都能在异常环境下关闭附加行为。

## 参考来源

- [Command Line Interface Guidelines (clig.dev)](https://clig.dev/)：TTY 检测、`--no-input`、`--dry-run`、退出码零/非零、stdout/stderr 分离、`NO_COLOR` 等通行 CLI 约定的权威出处。本文的人机双受众部分与它对齐；信封、固定退出码表、错误进 stdout 是在它之上为 Agent 场景做的加法，已在正文标注差异。
- [Writing effective tools for AI agents — Anthropic](https://www.anthropic.com/engineering/writing-tools-for-agents)：渐进式披露（`response_format` concise/detailed）、语义标识符优先于 UUID、大结果分页/落盘、错误作为 Agent 可执行的引导、名词-动词命名的实证依据。
- [Model Context Protocol — Tools 规范](https://modelcontextprotocol.io/)：错误放在结果对象而非协议层，供模型「看到」并推理——本文「失败也走 stdout 信封」的同构依据。
