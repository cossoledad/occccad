# Token-efficient Agent Architecture

> 状态：v1 current baseline
> 原则：Cheap by default, deep on demand.

本文是 occccad 面向 AI Agent 的上下文与验证架构。它取代并提炼了根目录原始 `save-token.md` 与 `save-token-plan.md`；未来维护本页、根/local `AGENTS.md` 与实际 CLI，不再把长篇任务提示词当作仓库知识源。

## 1. 目标与非目标

目标是让一次正确工程任务的无关 token 成本接近零，而不是让仓库文本最短：

```text
minimum irrelevant context
per correctly completed engineering task
```

保留完整知识、测试和诊断，但让它们可寻址、可选择、按失败展开。不得用删除测试、降低覆盖、隐藏错误、压缩领域语义、禁止深挖或引入重型 Agent framework 来换取更少输出。

需要分别治理四类成本：

| 成本 | 默认策略 |
|---|---|
| Persistent context | 根指南只保留 router 与全局不变量 |
| Discovery | 搜索符号和所有权后按范围读取 |
| Validation | 具体测试和领域 scope 优先，风险驱动升级 |
| Failure investigation | 成功安静，失败保留完整有用诊断 |

运行时 observability 不属于 Agent 输出压缩范围；生产/开发日志质量不能因此下降。

## 2. 上下文分层

```text
L0 root AGENTS.md
  -> L1 nearest local AGENTS.md / module README
    -> L2 relevant symbols, ranges and colocated tests
      -> L3 focused architecture, integration evidence and corpus
```

普通局部任务应停留在 L0 + 一个 L1 + 少量 L2。执行 SREA：

1. **Search**：用 `rg`/`rg --files` 确认所属模块、符号和测试；
2. **Read**：只读声明、实现命中区间和邻近测试；
3. **Expand**：只有直接 caller、状态生命周期或失败证据不足时扩大；
4. **Architecture**：只有跨长期语义时按 `docs/README.md` 定位章节。

满足以下条件即停止探索：所属模块已知、当前行为可解释、实现与测试已定位、最小改动及验证路径已明确。

主动升级到深层架构的典型触发器是 stable identity、Revision/history/Undo、persistent naming、单位/容差、数据库模型、公共协议、分布式一致性、Worker/service 边界或新 CAD 领域抽象。局部数学、渲染、组件或 adapter bug 通常不先读完整架构书。

## 3. 知识所有权

| 载体 | 只保存 |
|---|---|
| root `AGENTS.md` | 项目身份、全局不变量、路由、默认排除、升级原则 |
| local `AGENTS.md` | 领域不变量、入口符号、大文件策略、局部验证 |
| module README | 当前职责、公共接口、配置、运行与故障语义 |
| `docs/README.md` | architecture knowledge router |
| Current Architecture | 可由当前源码/迁移/测试证明的系统事实 |
| Target Architecture | 长期领域语义、跨模块边界与阶段门 |
| focused architecture | 面向单一横切主题的短投影；必须链接 canonical 来源 |
| Issue/会话 | goal、scope、known state、remaining、do-not-touch 等临时上下文 |

当前五个高频 local guide 是 Assembly、OCCT、Geometry Worker、Services 和 CAD Web。只有同时具备独立语义、独立验证、高频修改的目录才新增 guide，避免规则碎片化。

## 4. 默认排除与大对象

默认不读：build/dist/node_modules、日志/data、`services/gen`、models/corpus、锁文件、`go.sum`、compile database 和 STEP/BREP/GLB/3dreplay。它们不是禁止访问；依赖解析、codegen discrepancy、真实 corpus 或制品问题可以显式进入。

Proto source 是协议权威；生成代码是 output。正常路径是 proto → regenerate → compile → contract tests。

大型文本阈值只作信号：

- 超过约 40 KB：先搜符号再读区间；
- 超过约 80 KB：职责拆分候选；
- 超过约 120 KB：强候选。

只有职责稳定、Agent 高频进入、局部任务长期加载无关区域时才物理拆分。拆职责而不是平均切行，且必须保持行为测试。当前 P2 已完成：

- Workbench 的 Properties/History 与 tree projection/selection mapping 分别进入 `workbench-inspector.tsx`、`workbench-tree-model.tsx`；主 orchestrator 由 98.3 KB 降至约 76 KB；
- Workspace 的公共 model/view types、legacy command adaptation、parameter/dependency evaluation、evaluation persistence 分别进入 `model.go`、`legacy_commands.go`、`evaluation_projection.go`、`evaluation_persistence.go`；`service.go` 从约 138 KB 降至 111 KB，`model_core.go` 从 122 KB 降至 83 KB；
- `solver.cpp` 只完成算法/依赖分析，分阶段私有拆分计划记录在 `kernel/assembly/SOLVER_ALGORITHMS.md` §13，未修改数值实现。

## 5. 验证 API

`invoke test` 保持人类/CI 全量入口。Agent 使用：

```bash
invoke check                         # 等同 changed-file smart routing
invoke check --changed
invoke check --scope assembly
invoke check --scope geometry
invoke check --scope sketch
invoke check --scope workspace
invoke check --scope services
invoke check --scope web
invoke check --scope all
invoke check --scope assembly --match DirectedAngle
invoke check --scope web --match realtime
invoke check --changed --plan
invoke context-audit
```

`--match` 是 Level 1 精确验证，只允许一个非 `all` 的显式 scope：C++ 使用域前缀 + CTest regex，Go 使用 `-run`，Web 使用场景路径/文件名 substring。零匹配必须失败，不能形成假阳性。它会跳过该域的跨层集成和 production build；公共接口或行为完成后仍应升级到无 `--match` 的 module scope。

验证层级：

```text
Level 1 exact --match
  -> Level 2 module scope
    -> Level 3 affected integrations included by that scope
      -> Level 4 all
```

changed-file mapping 的核心规则：

| 改动 | scope |
|---|---|
| `kernel/assembly`、assembly corpus | assembly |
| `kernel/occt`、models | geometry |
| Geometry Worker sketch | sketch |
| Workspace/Model Core | workspace |
| 其他 Go、Go conformance | services |
| Web | web |
| Proto/generated、迁移、通用 Worker/`kernel/api`、共享 build/validation、未知路径 | all |
| Markdown | 无可执行验证 |

多域改动合并 scope；`all` 覆盖其他选择。路由只读取 Git 路径，不读取文件内容，并由 `tests/python/test_validation_routing.py` 固定。无法确定所有权时保守升级。

`--plan` 不执行任何构建或测试，只打印 selected scope、changed path 数、升级原因、cwd 与底层命令。修改 mapping、公共契约或准备运行昂贵验证时先审阅 plan。

## 6. 输出契约

默认执行器捕获每个子进程 stdout/stderr：

- success：每步一行 `PASS + elapsed`，最后一行总计；
- failure：终端保留有界首尾和 error/fail/expected/actual 等高信号行，完整 stdout/stderr、cwd 与命令写入 `build/agent-logs/`；
- `--verbose`：需要观察进度、warning 或卡顿时恢复流式输出。

Web scenario runner 支持多个 OR substring、`--list` 和 `--verbose`。每个场景仍在独立 Node 进程运行；成功只打印总数，失败 replay 该场景输出，无匹配返回非零。

`invoke context-audit` 是无索引服务的静态治理门：检查根/local guide 尺寸、必需 knowledge entry、Markdown 本地断链、已淘汰 prompt 残留、`tasks.py` 自身阈值，并统计 Git 已跟踪及未忽略文件中的 >40 KB 文本。成功只给 large/strong candidate 计数，`--verbose` 才列完整热点；热点本身不是失败，错误知识入口才失败。

## 7. 可复现指标

不估算模型内部 token，使用代理指标防止架构退化：

| 指标 | v1 基线/期望 |
|---|---|
| 根 persistent context | 65 行、约 5.6 KB；保持 router 量级 |
| 普通任务初始路径 | root + 1 local guide + 1–4 实现区间 + 1–2 tests |
| architecture 默认读取 | 普通局部任务为 0；语义升级时只读命中章节 |
| scoped validation | 不运行无关领域；共享契约自动升级 |
| full success output | 每个步骤一行；当前 `check all` 为约 9 行 |
| Web success output | 一行场景计数 |
| generated/corpus reads | 需要显式任务理由 |
| routing governance | `context-audit` 通过；新入口无断链且 root/local guide 不越界 |

典型静态演练：

| 任务 | 默认路径 |
|---|---|
| Directed Angle bug | root → Assembly guide → solver symbol/test → assembly `--match` → assembly scope |
| Front selection bug | root → Web guide → selection/tool/scenario → web `--match` → web scope |
| Workspace Undo bug | root → Services guide → history symbols/tests → workspace `--match` → workspace scope；必要时 Target 4.3 |
| Proto RPC addition | root → Worker + Services guides → proto/adapters → all；必须正式 Router 验证 |
| Sketch constraint bug | root → Worker/Web guides（按调用面）→ constraint symbols/tests → sketch scope |

若普通局部任务仍整读大 architecture/source、重复寻找入口、读取 generated output 或运行全仓，应优先修正所有权、router 或 validation mapping，而不是删除知识和测试。

## 8. 后续计划

当前路线状态：P0（根路由、排除、大文件协议）、P1（local guides、docs router、scope/match/changed/plan/quiet、Web filtering）和本轮 P2（Workbench/Workspace 职责拆分、focused projections、solver 拆分分析）已经落地。P2 不设“一次拆完整仓”的完成门，后续以真实任务的无关读取证据逐项推进。

按实测频率推进，不以文件数或行数为 KPI：

1. 为 `--changed` 增加新领域路径时同步路由单测；有跨层事故证据再细化 affected integration，不建立自定义 DSL。
2. 观察 `workbench.tsx` 拆分后的修改模式；只有 command dialogs 仍频繁造成无关读取时才继续拆分。
3. Workspace 剩余 `service.go`/`model_core.go` 与 viewport engine 仍是候选；继续按 service orchestration/typed handlers 和 scene/input/rendering 的真实修改证据拆分。
4. `solver.cpp` 严格按 Solver Algorithms §13 的 S0–S6 门推进，首个实施阶段只移动 equation semantics，不同时调算法。
5. 新增 focused projection 仅在 router 仍需频繁进入长文档时进行；projection 必须短、带 canonical section link、避免复制易漂移事实。当前只维护 model-history、persistent-naming、worker-contracts 与本页。
6. 长任务默认继续使用 Issue/会话；只有跨 session 丢失状态成为反复问题时才引入可替换的 `.agent/current-task.md`，任务完成即删除。

维护完成条件：入口可发现、scope 可选择、成功安静、失败详细、共享风险会升级、知识仍完整可寻址，并且没有为 TEAA 引入第二套构建系统、索引服务或隐式状态机。
