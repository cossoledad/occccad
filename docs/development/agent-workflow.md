# Agent 上下文与验证工作流

领域不变量归[根指南](../../AGENTS.md)，源码/测试按[项目地图](../README.md)查找。先 `git status --short`，保留已有改动；按 Search → Read → Expand → Architecture，明确所属模块、真实调用链、最小修改及验证后停止扩张。

## 最小上下文

```bash
mise tasks ls
mise run context --recipe property --json
mise run context --recipe selection --symbol AssemblyDialogLifecycle --read --json
```

[配方](../../dev/recipes.json)仅保存 property/rpc/selection/feature/diagnostic 工程入口和验收义务，不拥有领域真相。`--read` 最多两段各 5 KB；返回搜索次数、源码字节和耗时，模型 token 未测量时为空。旧批量 workflow-baseline 包装已合并为 context 输出与下述 check 报告；需要比较时串行执行相同配方/选择范围，不能把一次样本当稳定分位数。

## 同一开发环境

先用 `mise run doctor`，需要实际运行身份时用 `--live`，已有浏览器用 `--scope web --browser`。doctor 不迁移或清库、不安装依赖。Ninja dry run 遇到必须执行的 glob/configure 检查时，freshness 明确记为可选 NOT_RUN；工具/资源就绪不替代实际 build/check，--live 另验证运行代码。资源与加载规则见[环境](environment.md)。工具未准备时执行 `mise run setup --component tooling`，不要为了帮助或 plan 构建整个 CAD。

## 渐进验证 API

```bash
mise run --quiet plan --changed --level unit --json
mise run check --changed --level unit --jobs 4
mise run check --scope assembly --level unit --match DirectedAngle --jobs 4
mise run check --scope workspace --level integration --match '^TestHistoryCapabilitiesAcrossTwoUndoAndRedoSteps$'
mise run check --scope web --level unit --match assembly-edit-intent
mise run --quiet plan --base HEAD~1 --head HEAD --level integration --json
```

优先最小 target/scope，不默认全库测试。`plan` 准备工具后只读：不安装、配置、构建、迁移或启动服务。`--json` 的 stdout 是单个 JSON，进度在 stderr；顶层可加 `--quiet`。Usage 声明用于 mise 发现与补全；任务使用已验证的 raw_args 原样转发给严格 CLI，--help 由 CLI 输出。这样缺值不会被 mise 的 Usage 归一化吞掉并回退默认构建。所有组合由 CLI 校验，未知参数、非法 jobs、零匹配均失败。

[validation.json](../../dev/validation.json)拥有路径归属、scope、level、叶任务和资源要求。默认/changed 包含 staged、unstaged、untracked、删除与 rename 两端，采用 NUL 分隔支持空格/换行。`--staged` 仅 index；`--base B --head H` 比较 `merge-base(B,H)..H`，不混入工作区；只给 base 则另包含工作区变化。缺引用/历史报错。

Proto、迁移、共享构建和未知路径保守扩大建议范围并说明原因。docs-only 实际执行 docs:check；干净工作区返回 NOT_RUN，不能证明当前提交通过。显式 scope 可缩小本次执行，但不能消除风险要求的未执行证据。

| level | 范围与资源边界 |
|---|---|
| unit（默认） | CTest、Go 不带 integration tag、Node scenarios、文档/工具检查；测试自有短期 SQLite/RPC 对象允许，禁止配置业务资源 |
| integration | `-tags=integration -count=1`；当前 Worker、RPC、数据库、ArtifactStore；按 scope 注册真实链路 |
| browser-mock | 已有浏览器与隔离前端，不能证明真实后台 |
| browser-live | 当前应用栈、真实资源与浏览器，运行身份单独核对 |
| full | 注册表中所选 scope 的上述各层；不代表所有后端、corpus、平台或任意样本 |

`--match` 仅支持一个明确的可筛选 scope：C++ 为所属前缀中的 CTest regex，Go 为 -run，Web 为路径 substring；不支持 full/浏览器/docs/tooling。精确 Web 场景仍保留生产构建。unit 不偷偷启动集成或浏览器；计划/报告列出 outstandingEvidence。通过只表示请求范围通过。

## 结论、日志与版本边界

PASS 是实际执行断言成功；FAIL 包含编译/断言/进程失败和零测试；ENVIRONMENT_BLOCKED 包含缺资源、必需 skip；NOT_RUN 是未执行或无选择；NOT_APPLICABLE 仅用于明确其他后端/可选证据；SOURCE_CHANGED 是验证期间相关源码或制品变化。父测试 PASS 不能覆盖子测试 skip。

Go unit 可使用原生缓存，报告 cached 与真实用例终态；集成强制重跑。CTest 每次使用新 JUnit，Node 需要实际场景数量，合同需要精确 case marker。合同/工具负例不是产品能力验收，声明覆盖不等于执行证据。

[Go 工具](../../services/internal/devtool/)将实际任务 argv/cwd、原生命令、计数、跳过、耗时、源码/dirty/untracked 身份、profile、相关制品、日志与复现命令写入 `build/agent-check/<run>/report.json`。完整输出脱敏后写盘，普通终端给摘要、失败上下文和日志路径。成功为零、失败非零；mise 会包装底层失败，不承诺透传精确退出码，分类以报告为准。工具日志在 `build/logs`。

## 从诊断到复现

```bash
mise run diagnose --document <documentId>
mise run diagnose --reference 'CAD_DIAGNOSTIC <documentId>/<recordId>' --json
mise run diagnose --reference 'CAD_DIAGNOSTIC <documentId>/<recordId>' --attempt 0 --replay
mise run diagnose --job <jobId>
```

从配置日志根定位归档，或用 --file；检查身份、符号链接、schema 与 8 MiB 预算。只导出失败时保留的数值请求到 `build/diagnostics`，由同一生产 [3dreplay](../../services/cmd/occccad-3dreplay/README.md)执行；不覆盖原文件、不替换新 Head。进程执行成功和观察到的数值失败分别报告。缺数值输入明确不可重放，不补造证据。

## 测量与维护

`mise run docs:check` 检查链接、锚点、围栏、导航、目录、指南预算、配方和大文本热点；审计变更运行 `mise run contract:check`。context/check 的机器结果可用于同范围串行比较，字节不是 token，局部耗时不是端到端开发速度。性能用 `mise run benchmark` 或 `benchmark:baseline`，保留优化构建检查、样本完整性、inclusive 阶段与内核/服务边界。

临时清单和验收原始记录归 `build/`，长期文档只保存当前规则。没有执行不能写通过，缺资源不能改用 Mock 制造通过；不因普通失败 clean，不自动下载浏览器，不引入 CI。
