# occccad Agent Router

本指南适用于整个仓库，只保留全局不变量与任务路由。进入高频领域后先读最近的 `AGENTS.md`；架构知识按 [docs/README.md](docs/README.md) 定位，Agent 上下文/验证架构见 [focused TEAA](docs/architecture/agent-efficiency.md)，不要默认通读两份大型架构文档。

## 项目与事实边界

occccad 是早期可运行的云原生参数化 CAD 垂直切片。参数模型是业务真相，精确 B-Rep、Mesh、缩略图和分析结果是带 provenance 的可重建制品。长期目标不代表当前已经交付；当前事实以代码、迁移、测试和 `docs/CURRENT_ARCHITECTURE.md` 为准，目标语义以 `docs/TARGET_ARCHITECTURE.md` 为准。

开始前检查 `git status --short`，保留用户或其他 Agent 的无关改动。准确版本和协议看锁文件、迁移与 Proto，不凭文档猜测。

## 渐进式上下文

遵循 SREA：Search → Read → Expand → Architecture。

1. 用 `rg`/`rg --files` 定位所属模块、符号和邻近测试；
2. 只读命中范围、调用方和测试，不先打开整个大文件；
3. 跨越稳定身份、Revision/Undo、数据库、公共协议、Worker 边界或新 CAD 领域时，再按 docs router 读取对应架构章节；
4. 已知所属模块、当前行为、实现、测试、最小变更和验证入口后，停止扩张上下文。

默认不读 `build/`、`node_modules/`、`dist/`、日志、`services/gen/`、`models/`、锁文件、`go.sum`、`compile_commands.json`、`*.step`、`*.brep`、`*.glb`、`*.3dreplay`。它们不是禁区；仅在依赖、生成 API、corpus 或制品问题明确需要时读取。协议先读 `proto/`，生成代码只用于 codegen/API 差异诊断。

超过约 40 KB 的文本先查符号再读区间；超过约 80 KB 仅在职责边界确实需要时扩大。不要为缩短文件而机械拆分。

## 仓库路由

| 任务 | 先读 |
|---|---|
| Assembly solver、DOF、约束运动 | `kernel/assembly/AGENTS.md` |
| OCCT、拓扑命名、精确几何 | `kernel/occt/AGENTS.md` |
| Geometry Worker、Proto/RPC、PlaneGCS | `workers/geometry/AGENTS.md` |
| Go 命令、Revision、Jobs、API、数据库 | `services/AGENTS.md` |
| React、工具、选择、Three.js、场景测试 | `web/apps/cad/AGENTS.md` |
| 跨模块 conformance | `tests/README.md` |
| 运行、进程和仓库概况 | `README.md` 与所属可执行单元 README |

## 平台不变量

- 持久引用使用 typed stable ID、InstancePath、Publication 或 PersistentSelection；不保存数组位置、显示名、OCCT local ID、指针或 mesh 序号。
- Revision 不可变，Workspace 显式可变；持久写入经 versioned Domain Command、幂等 Transaction、ChangeSet 和 CAS。Undo/Redo 使用补偿/重放，不抹除历史。
- 区分 UI Command、Interaction/Preview、Domain Command 和 Compute Job。鼠标轨迹不形成 Revision；预览不成为业务真相。
- 单位、容差、分支意图和失败诊断显式。缓存键包含规范输入、依赖快照、evaluator/kernel/solver 版本和策略 digest。
- Worker 接收不可变粗粒度输入且不持有业务数据库真相；第三方类型不得泄漏到领域 Proto 或持久模型。
- 至少一次投递按效果幂等；Job 有 attempt/lease/deadline/cancel，迟到结果不能覆盖新 Head。
- 浏览器只负责交互、近似预览和显示；提交后由权威 evaluator 验证。
- 输入文件、插件、表达式和 Worker 请求均不可信，必须限制资源、权限、网络与副作用。

## 当前开发数据边界

项目尚未发布且没有外部持久数据承诺。需要统一 schema、命令、Revision 或 evaluator 语义时直接修正唯一实现及调用方，不为实验数据增加 adapter/双写。未发布迁移可重写，并从空库验证。

仅可用 `invoke data.reset --yes` 或 `invoke run.app --reset-data` 删除命令明确报告的 `occccad` schema 与本地 ArtifactStore；先停止占用进程，并在交付中说明。不得扩大到其他 schema、数据库、目录或外部存储。维护者宣布发布基线或必须保留的外部数据后，此豁免终止，迁移只追加且旧 Revision 必须可读。

## 变更与验证

在任务范围内完成调用链、根因、测试和受影响文档的闭环；不顺手重写模块、升级全仓依赖或改变部署拓扑。新增外部服务、重大依赖/许可证、公开兼容承诺，或改变稳定身份、历史、单位、拓扑引用、跨文档一致性时先澄清。

默认运行最小能证明正确性的验证：`invoke check --scope <domain> --match <test>` → 无 `--match` 的领域检查 → 受影响集成 → `invoke check --scope all`。不确定路由时先用 `--plan`。`invoke check` 默认按工作区改动保守选域，成功只给摘要，失败给高信号诊断、完整日志和复现命令；`--verbose` 可流式显示。`invoke test` 保持全量人类/CI 入口，Agent 知识入口变化后运行 `invoke context-audit`。

公共 Proto、数据库迁移、共享构建系统和无法识别所有权的改动必须升级。复杂 Web 视觉/交互除测试和构建外还需重启浏览器验收；若环境不允许，明确未验证项，不虚报通过。

## 文档与交付

已实现事实变化更新 `docs/CURRENT_ARCHITECTURE.md`；长期语义变化更新 `docs/TARGET_ARCHITECTURE.md`；可执行单元职责、接口或运行方式变化更新所属 README。AGENTS 只放稳定行为、路由和验证，README 放模块职责/运行，Architecture 放领域语义/跨模块决策，临时任务状态不写入长期指南。

交付时先写结果，再写关键判断、变更文件、实际验证和限制。区分已实现、设计、候选与未验证假设。
