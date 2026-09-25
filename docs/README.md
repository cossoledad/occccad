# 文档导航

普通修复先读最近的 `AGENTS.md`、实现和邻近测试；跨身份、历史、单位、数据库、公共协议或 Worker 边界时再进入对应架构分册。

## 入口与职责

| 要回答的问题 | 唯一入口 |
|---|---|
| 当前代码实现了什么、有哪些限制 | [当前架构](CURRENT_ARCHITECTURE.md) → `architecture/current/` |
| 长期领域语义和平台边界是什么 | [目标架构](TARGET_ARCHITECTURE.md) → `architecture/target/` |
| 下一步做什么、主线和支线如何依赖 | [统一路线](../plans/README.md) |
| 如何运行、配置、排障 | [根 README](../README.md) 与所属可执行单元 README |
| 如何准备环境、复用开发数据库和浏览器依赖 | [环境准备](development-environment.md) |
| Agent 如何获取上下文和验证 | [Agent 路由](../AGENTS.md)、[focused TEAA](architecture/agent-efficiency.md) |

## 按领域直达

| 领域 | 当前事实 | 目标契约 |
|---|---|---|
| 拓扑/进程 | [运行边界](architecture/current/runtime.md) | [系统边界](architecture/target/system.md)、[计算部署](architecture/target/compute-boundaries.md) |
| Command/Revision/Undo | [模型与历史](architecture/current/model-history.md) | [命令与历史](architecture/target/commands-history.md) |
| Parameter/Dependency | [参数与求值](architecture/current/model-history.md) | [参数与增量](architecture/target/parameters-evaluation.md)、[接口与治理](architecture/target/model-governance.md) |
| Sketch | [Part/Sketch](architecture/current/part-sketch.md) | [模型](architecture/target/sketch-model.md)、[编辑](architecture/target/sketch-editing.md)、[Solver](architecture/target/sketch-solver.md)、[集成](architecture/target/sketch-integration.md) |
| Part Feature | [Part](architecture/current/part-sketch.md) | [模型](architecture/target/part-model.md)、[扩展](architecture/target/part-extensions.md)、[求值门](architecture/target/part-evaluation.md) |
| Part 几何/显示数据 | [几何制品](architecture/current/geometry-representations.md) | [大模型数据面](architecture/target/large-models.md) |
| Naming | [命名/重连](architecture/current/persistent-naming.md) | [拓扑命名](architecture/target/persistent-naming.md) |
| Product/Assembly | [Product](architecture/current/product-assembly.md) | [上下文](architecture/target/product-context.md)、[六类约束](architecture/target/assembly-constraints.md)、[装配](architecture/target/assembly.md)、[求值](architecture/target/product-evaluation.md) |
| Surface/3D Wire | 尚无完整实现 | [模型](architecture/target/surface-model.md)、[Feature](architecture/target/surface-features.md)、[质量](architecture/target/surface-quality.md)、[集成](architecture/target/surface-integration.md) |
| DMU/Kinematics | 尚无完整实现 | [候选合同](architecture/target/kinematics-dmu.md) |
| 大文件导入/原生大模型 | [交换现状与限制](architecture/current/jobs-artifacts.md) | [工作集与导入设计提案](architecture/target/large-models.md)、[实施支线](../plans/import-large-models.md) |
| Jobs/Artifact/通信 | [Jobs 与制品](architecture/current/jobs-artifacts.md) | [分布式平台](architecture/target/distributed-platform.md) |
| Web | [交互架构](architecture/current/web.md) | 对应领域交互合同与[命令分层](architecture/target/commands-history.md) |
| 安全/验证/交付 | [验证边界](architecture/current/validation.md) | [质量与扩展](architecture/target/quality-extensions.md)、[交付治理](architecture/target/delivery-governance.md) |

## 高频横切摘要

[Model/History](architecture/model-history.md)、[Persistent Naming](architecture/persistent-naming.md)、[Part 求值与 Sketch 上下文](architecture/part-feature-evaluation.md)、[Worker Contracts](architecture/worker-contracts.md) 是短导航与跨模块约束摘要；它们链接对应分册，不另设事实基线或计划。

商业 CAD 对照页定位保留在[工作流参考索引](references/cad-workflows.md)，不作为当前能力证据。

## 维护规则

- `AGENTS.md` 只放稳定行为、路由、验证；README 放职责/运行；当前分册放实现事实与代码/测试入口；目标分册放仍有效契约；计划只放未完成工作。
- 完成计划先归入事实和长期契约，再删除原条目；剩余验收单独保留。历史讨论用 Git 追溯，不复制归档计划。
- 分册按职责边界拆分；超过约 40 KB 先检查是否混入其他领域或实施日志。不要为了篇幅缩短而删除稳定身份、失败、分支和质量门。
- 旧架构章节号保留为语义检索标识；新增链接使用具体分册，不再指向大文件行号。开发任务使用领域标识，只有统一路线定义依赖和状态。
- 修改入口后运行 `invoke context-audit`；普通局部任务已明确实现、测试与最小验证入口时停止扩展上下文。
