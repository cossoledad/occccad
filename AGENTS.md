# occccad AI Agent Guide

本文件适用于整个仓库，面向任何参与分析、设计、编码、测试、审查和文档维护的 AI Agent。它提供长期方向与工程判断框架，不是逐条执行的固定脚本。更深目录若未来增加 `AGENTS.md`，可以细化局部规则，但不得静默违背这里的平台不变量。

## 1. 项目使命

occcad 是一个基于开源技术的云原生、分布式参数化 CAD 与产品研发平台。长期能力以 CATIA 的工程广度和严谨性为参照，但不复制其 UI、私有格式或内部实现。平台应支持从 Sketch、Part、Surface 到 Product/Assembly、DMU、Simulation 和工程扩展的连续工作流，并保持开放协议、可替换计算后端和可自托管部署。

这不是“把桌面 CAD 放到远程桌面”，也不是一组无状态几何 API。核心设计资产是可版本化、可重放、可协作的参数模型；精确 B-Rep、Mesh、缩略图和分析结果是带 provenance 的可重建制品。

仓库当前仍是早期可运行垂直切片。不要因为目标宏大而假装功能已经存在，也不要因为当前实现简单而把临时结构固化成长期架构。

## 2. 开始工作时建立正确上下文

先阅读与任务最相关的少量材料，而不是盲目扫描或凭常见模式重写：

- [README.md](README.md)：项目状态、仓库布局和通用运行方式；
- [docs/CURRENT_ARCHITECTURE.md](docs/CURRENT_ARCHITECTURE.md)：可以从当前代码验证的事实；
- [docs/TARGET_ARCHITECTURE.md](docs/TARGET_ARCHITECTURE.md)：长期领域语义、平台不变量、详细设计和开发治理；
- 被修改可执行单元的 README、邻近实现与测试；
- 锁文件、迁移和 Proto 才是具体版本与协议事实。

先检查工作区已有改动。它们可能属于用户或其他 Agent；保留无关改动，避免大范围格式化、回滚或覆盖。若任务描述与代码现状不一致，先用代码、测试和运行证据确认差距，再决定是实现、迁移还是只修正文档。

## 3. Agent 的自主空间

在任务范围内主动完成有价值的闭环：理解调用链、修正根因、补充测试、更新受影响文档并验证结果。无需为可逆、局部、符合现有架构的正常工程选择逐项请求许可。

可以并且应该质疑不必要的复杂度。若目标文档中的建议在当前阶段明显过重，选择更小的兼容切片，并说明它如何保留演进路径。若实践证据表明目标设计本身需要改变，不要用旁路掩盖冲突；提出更优方案，并同步更新架构基线。

### 3.1 当前开发期的数据与兼容策略

仓库当前处于未发布、没有外部持久数据承诺的从零开发阶段。此阶段的 PostgreSQL `occccad` schema、本地 ArtifactStore、任务、会话、缓存与历史样例都属于可丢弃的开发数据，不构成兼容基线。开发中的正确性、领域语义唯一性和架构整洁优先于保留这些数据。

- 核心 schema、命令、Revision、evaluator 或身份语义需要调整时，直接修正当前设计与所有调用方；不要为实验数据增加 adapter、双写、回填、兼容分支或第二套状态机。
- 未发布迁移允许修改、合并、重排或替换。语义变化后应清空开发数据，从空库执行完整迁移，并重跑当前端到端场景与 conformance 测试；不得用旧样例“还能读取”替代新基线验证。
- 需要重建本地数据时，Agent 可以运行仓库提供的显式开发重置命令，无需为保留开发数据停下来询问；但必须只命中命令报告的固定 schema 和制品目录，不能扩大到整个数据库、其他 schema、仓库目录或外部存储，并在交付中说明已删除内容。
- `invoke run.app --reset-data` 是清空全部服务端开发数据、重建 schema 并启动应用的标准入口；`invoke data.reset --yes` 只执行清理和迁移。执行前应停止仍在使用这些数据源的 occccad 进程。

一旦维护者明确宣布首个发布基线、接入必须保留的外部数据，或承诺公共协议兼容，上述豁免立即终止。从该边界开始，迁移只追加、旧 Revision 只读、命令 upcast/adapter 与升级验证成为强制要求。任何 Agent 都不得自行把该发布边界提前或推迟。

需要停下来澄清的情况主要包括：

- 用户意图存在会显著改变产品语义的多种解释；
- 操作会删除或不可逆迁移已发布或外部用户数据；当前 §3.1 明确为可丢弃的本地开发数据除外；
- 需要新增外部服务、重大依赖、许可证承诺或公开兼容性；
- 需要改变稳定身份、历史、单位、拓扑引用或跨文档一致性；
- 缺少凭据、外部决策或无法从仓库推断的业务规则。

自主不等于扩大范围。不要顺手重写不相关模块、升级全仓依赖、改变部署拓扑或“顺便修复”大量旧问题。

## 4. 平台不变量

所有实现都应尊重以下原则。更完整语义见目标架构。

1. **参数模型是源，几何是缓存**：业务真相不能只存在浏览器、Worker 内存、Redis 或 B-Rep 文件。
2. **稳定身份优先于数组位置和显示名称**：持久引用使用 typed ID、InstancePath、Publication 或 PersistentSelection；不得保存临时 OCCT local ID、指针、三角形序号或数组下标作为长期身份。
3. **不可变 Revision，显式 Workspace**：提交通过 versioned Domain Command、幂等 Transaction、ChangeSet 和 CAS；迟到结果不能覆盖新 Head。
4. **Undo 不抹除历史**：持久 Undo/Redo 使用可验证的补偿/重放语义；Restore、Branch、历史查看和 Feature Rollback 各自独立。
5. **单位、容差和分支意图显式**：不依赖隐式单位、Worker 默认值或“选择第一个解”。非有限数、量纲错误和拓扑歧义必须诊断。
6. **确定性与 provenance**：缓存键包含规范输入、依赖快照、evaluator/kernel/solver 版本和策略 digest；相同输入应得到语义等价结果。
7. **粗粒度远程，细粒度本地**：网络边界依据负载、故障、安全和扩缩容差异，而不是领域名词数量。不要把每个 OCCT 调用或 Feature 变成 RPC。
8. **开放契约、隔离第三方类型**：领域 Proto 和持久模型不泄漏 OCCT、求解器、数据库或对象存储内部类型。
9. **失败必须可解释**：区分用户模型失败、数值/拓扑失败、冲突和基础设施失败；允许可修复的失败 Revision，但不能用旧几何冒充当前成功结果。
10. **至少一次 + 幂等**：消息、任务和重试按效果唯一设计，不假设 exactly-once。
11. **发布后旧数据可读**：达到 §3.1 的发布边界后，schema、命令和 evaluator 演进使用版本、adapter、shadow diff 和 corpus；已发布迁移只追加，不原地篡改。当前未发布开发数据不享有该兼容承诺。
12. **安全默认**：导入文件、插件、表达式和 Worker 输入均不可信；限制资源、权限、网络和副作用。

## 5. 面向模块的工程判断

### 5.1 领域与命令

- 先定义用户意图、稳定实体、状态、不变量和失败语义，再选择存储表或类层次。
- 区分 UI Command、临时 Interaction/Preview、持久 Domain Command 和 Compute Job。
- 一个用户动作形成一个可理解的 Transaction；不要把鼠标轨迹写成数百个 Revision。
- Command Handler 应是确定、无 I/O 的模型变换；昂贵求值在事务外执行，最终短事务 CAS 提交。
- 新命令使用可版本化 typed payload 和 handler registry；不要继续扩张跨领域的巨型可选字段结构或 switch。
- Command Handler 产生的是求值前候选模型；若 Solver、Evaluator 或规范化步骤会改写坐标、诊断、参数或派生字段，ChangeSet 的 `after` 必须在最终 Revision 确定后重新生成。不得让 ChangeSet 描述一个从未持久化过的中间状态。
- 新增 PropertySlot 时必须同时贯通：handler ChangeSet、当前值读取、补偿值回写、digest/canonicalization、依赖种子以及 Undo/Redo 测试。只会“写新 Revision”不代表该属性已经接入历史系统。

### 5.2 参数化与几何

- 可驱动字段应有稳定 PropertySlot/ParameterId、值类型、单位、source 和 dependency edges。
- 普通表达式必须纯、受限、可静态检查；闭环只存在于明确的 Sketch、Assembly、Optimization 等 Solver Domain。
- 修改参数时验证 dirty closure；增量求值必须与清缓存后的全量求值语义等价。
- OCCT 负责精确几何算法，不负责定义产品领域、命令历史或跨文档身份。
- 对退化几何、多个解、容差边界和算法失败建模，不把 `IsDone()` 当作全部业务验证。

### 5.3 服务与分布式计算

- Go 控制面优先保持模块化单体。只有独立扩缩容、故障隔离、安全边界或发布节奏有持续证据时才拆服务。
- Worker 接收不可变输入 manifest，通过对象存储交换大制品，不持有业务数据库凭据。
- Job 必须有状态机、attempt/lease、deadline、cancel、资源限制、幂等提交和迟到结果保护。
- 本地开发可以用文件存储和单机 Router，但生产语义不能依赖共享本地目录或进程内注册表。
- Proto 新增 RPC 后必须检查并测试每一层实现：Worker server、生成代码、语言适配 client、GeometryPool/Router 代理和 capability 报告。直接连 Worker 成功不能证明经正式 Router 路径可用；任一代理遗漏都会在运行时表现为 `Unimplemented`。

### 5.4 Web 与交互

- 浏览器负责交互、预览和显示，不是权威 B-Rep 或权限来源。
- 页面/功能层不要直接散布 Three.js 生命周期和全局事件；复用 Viewport、Input、Tool、Selection 和 Command 边界。
- 服务端状态与短期交互状态分离。相同 GeometryId 的实例共享资源，大装配按结构、LOD 和可见性渐进加载。
- 交互预览可以近似，但提交后必须由权威 evaluator 验证，并清楚标识 stale/preview/failed 状态。
- Pointer 工具必须显式拥有完整手势生命周期：`pointerdown`、移动、`pointerup`、`pointercancel`、capture/release、lost capture、窗口失焦和 Esc。两点/两选择工具的第一次点击状态必须跨第一次 `pointerup` 保留，且不能让 Selection 或 Navigation 接管配对事件。
- 点、线、面、辅助标记应使用与其语义一致的 Three.js primitive；遍历 Group 时不得假设每个 Object3D 都有 material。点的像素尺寸、depth test/render order 和草图平面变换需要单独验证，不能用“线可见”推断“点也可见”。
- 纯展示样式不强制单元测试；输入状态机、命令生成、选择引用、渲染数据分层和历史按钮 capability 等确定性逻辑必须有轻量测试。复杂跨层交互再补组件或 Playwright 场景。

### 5.5 Proto、数据库与兼容

- 当前未发布阶段可以直接统一 Proto、数据库与命令契约，并同步修改所有调用方；一旦越过 §3.1 的发布边界，Proto 字段只追加、删除字段号保留。
- 当前未发布迁移可以重写并以空库验证；发布后迁移只追加，使用 expand/migrate/contract，大回填使用可恢复任务，不放在长部署事务中。
- PostgreSQL 保存业务关系和事务，S3 兼容存储保存不可变大制品，Redis/Valkey 只保存可丢失状态。
- 发布后采用读旧写新：旧 Revision 不原地改写，adapter 的输出必须有确定性测试；当前开发数据直接重建，不维护实验 adapter。

## 6. 实施一个变更

根据风险调整深度，通常遵循以下思路：

1. 定位用户场景、当前调用链、对应目标架构章节和真正的最小变更面；
2. 先识别不变量、当前发布边界和失败条件，再选择最简设计；
3. 优先复用已有领域边界，删除重复概念，而不是增加并行框架；
4. 以薄的端到端切片实现，贯穿命令、模型、求值、API/UI 和测试中确有需要的部分；
5. 对风险最高的假设先做小型 spike 或基准，但不要把实验类型泄漏到公共契约；
6. 验证正常、边界、失败和重试/并发；当前阶段验证空数据重建，发布后再验证旧数据路径；
7. 审查 diff 是否包含无关改动、隐藏全局状态、未版本化协议或无法解释的复杂度；
8. 更新事实文档和服务 README，并清楚报告已完成、验证范围和仍存在的限制。

不要为了展示工作量制造抽象。小函数、明确数据流和少量稳定接口通常优于通用但未经需求验证的框架。也不要用临时 JSON、字符串枚举和注释替代真正需要长期稳定的领域契约。

## 7. 验证策略

验证应与风险成比例，并尽量从最相关层开始：

- Go：在 `services/` 下运行相关包测试，较大控制面改动再运行 `go test ./...`；
- C++/几何：使用 `invoke configure`、`invoke build` 和 `invoke test`，可用 `--target`/`--filter` 缩小反馈环；
- Web：至少运行 `invoke web.build`，复杂交互应补单元/组件/Playwright 场景；
- Proto/跨语言：重新生成受影响代码并验证 Go/C++ capability；发布后再覆盖协议兼容行为；
- 数据库：当前阶段验证空库重建、重复执行和校验和；发布后增加已有库升级及新旧应用重叠窗口；
- CAD 算法：优先 conformance、golden、metamorphic、fuzz 和退化 corpus，而不只验证一个示例模型；
- 分布式路径：覆盖重复投递、Worker crash/timeout、CAS 冲突、取消、对象上传后提交失败和 projection 重建。

若环境无法运行完整验证，完成能运行的最有价值部分，并明确未验证项和原因。不要声称未实际运行的测试通过，也不要为了通过测试而降低领域不变量。

### 7.1 跨层功能的防漏回归清单

以下检查来自仓库中已经实际发生过的故障。它们不是临时问题记录，而是新增命令、Sketch/Feature、Worker RPC、实时通道和输入工具时的长期完成条件。

| 故障信号 | 容易遗漏的根因 | 后续强制检查 |
|---|---|---|
| `digest mismatch` / `no longer has the original after value` | ChangeSet 在权威求值前生成，或新增 PropertySlot 没有接入历史投影 | 对最终持久 Revision 重建 ChangeSet；测试 evaluator 改写字段后的 Undo 和 Redo；确认 slot 能读取、回写和 canonicalize |
| 第二次 Undo/Redo 消失，或 `nothing to undo for this actor` 与按钮状态不一致 | capability 与执行路径使用了不同的历史折叠规则，或把 redo 栈当成单值状态 | 用同一 action-log 语义计算 `canUndo/canRedo` 和执行目标；覆盖两次 Undo、两次 Redo、新提交截断 Redo、不同 actor 和刷新后恢复 |
| `edge ... references an unknown node` | Feature、依赖边和节点不是从同一最终模型原子构建，Undo 删除上游后仍残留边 | 每次提交从最终模型验证全部 edge 两端；覆盖删除/补偿上游、dirty closure 和清缓存冷重建 |
| RPC 返回 `Unimplemented` | Proto/Worker 已实现，但 Router、Pool 或代理 server 没有转发 | 通过应用实际连接的 Router 做 RPC 测试；枚举并核对完整 GeometryWorker service method/capability |
| WebSocket 握手 403 | 只测试了 API 直连，遗漏开发服务器 Upgrade 代理、Origin、cookie/CSRF 或 subprotocol | 同时测试 API 直连和浏览器实际入口（例如 Vite `/api/realtime`）；验证 Upgrade headers、允许 Origin、session、CSRF 和 `occccad.realtime.v1` |
| 第一次点击抬起后预览/绘制中断 | 工具只处理 `pointerdown/move`，配对的 `pointerup` 落入 Selection/Navigation，或 capture 被意外取消 | 用真实 `down → up → move → down → up` 序列测试 Point、Line、Rectangle 和双选择 Constraint；另测 cancel、lost capture、Esc 与中键导航抢占 |
| 草图点存在于模型但不可见 | Point 被塞进 LineSegments、材质/深度/尺寸不适合，或 Group 遍历假设 material 存在 | 分离 point/line render model 和 primitive；用包含独立 Point、Line、端点及诊断色的 fixture 验证，至少执行 TypeScript 检查和生产构建 |

一个新的 Sketch/Feature 垂直切片不能只以“创建成功”作为完成标准。与变更相关时，至少串行验证：进入/退出编辑模式、预览、提交、权威求值、刷新重载、依赖图、Pad/下游消费、连续两步 Undo/Redo、按钮置灰，以及通过正式 Router/浏览器代理的真实通信路径。开发数据可按 §3.1 重置，但清库不能替代同一模型在完整状态转换中的正确性验证。

## 8. 代码与依赖质量

- 遵循邻近代码的语言惯例和 `.editorconfig`、`.clang-format`、`.clang-tidy`；避免全仓格式化。
- 命名表达领域语义；注释解释原因、边界和非显然约束，不复述代码。
- 公共 API 保持小而 typed；错误包含稳定领域 code 和足够上下文，日志不泄漏模型内容、凭据或 signed URL。
- 新依赖只有在明显降低风险/复杂度且自身可治理时引入。记录许可证、版本、维护状态、替代方案和适配层；优先使用当前依赖栈。
- 优化以测量为依据，但 CAD 正确性、稳定身份和可重现性不能为局部性能捷径让路。

## 9. 文档责任

本仓库把文档视为实现的一部分：

- 已实现事实变化：更新 `docs/CURRENT_ARCHITECTURE.md`；
- 长期语义、边界或选型变化：更新 `docs/TARGET_ARCHITECTURE.md`；
- 可执行单元的职责、接口、配置、运行或故障语义变化：更新对应 README；
- 图使用 Mermaid；`docs/` 默认只保留现有架构和目标架构两份核心文档；
- 不创建按 Demo、版本或临时讨论命名的架构文档来逃避整合；
- 外部库的“候选”不等于已依赖，“目标”不等于已交付。

目标架构第 19 章给出了新模块设计模板、Definition of Ready/Done 和架构变更流程。复杂模块以它为检查面，但要根据实际问题裁剪，避免文档驱动的形式主义。

## 10. 禁止的捷径

- 不把 Worker 缓存、本地文件、浏览器 Store 或 Redis 当唯一数据源；
- 不把 OCCT 类型、瞬时 topology ID 或 mesh primitive ID 写入持久业务协议；
- 不在数据库事务中等待长时间几何/网络计算；
- 不用随机重试掩盖非幂等写入或并发覆盖；
- 不静默选择拓扑、约束、Loft、Sweep 或装配多解的“第一个”；
- 不直接修改已经发布的数据库迁移、Revision 或历史 payload；
- 不仅因为未来可能需要就拆微服务、引入消息系统、插件框架或通用 DSL；
- 不仅为了快速演示就在新路径绕过 Command、权限、版本、单位、诊断和测试；
- 不宣称达到 CATIA 级能力，除非相应领域的语义、诊断、规模和验证门都已满足。

## 11. 交付沟通

完成工作时先陈述结果，再给出重要设计判断、变更文件、验证证据和已知限制。区分已经实现、仅设计、候选技术和未验证假设。若发现超出本次范围但重要的问题，指出其影响和建议优先级，不擅自扩大变更。

最好的贡献不是代码最多，而是在保持用户意图与当前发布边界的前提下，用最少必要机制交付一个清晰、可靠、可继续演进的能力。
