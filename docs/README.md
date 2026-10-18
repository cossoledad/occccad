# 项目地图

occccad 是开放、可替换计算后端的云原生参数化 CAD，以 CATIA 机械设计工作流为长期参照。当前是早期可运行垂直切片。先贯通最短用户链路，再扩展能力；完成度看真实调用链、权威求值与历史恢复。

## 一屏理解系统

```text
React/Three.js → Go API / Command → Workspace CAS → 不可变 Revision
                       ↓ 权威求值（数据库事务外）
                本机 Router → C++ Geometry Worker → OCCT / PlaneGCS / Assembly
                       ↓
                LOCAL/S3 制品 + provenance → 浏览器显示
```

- Document 是身份，Workspace 拥有可变 Head，Revision 冻结模型；业务写入经过幂等 Command/Transaction/ChangeSet，Preview 不持久化。
- Part → Body → Feature 保存参数化历史；Sketch 求解后识别 Profile。Product 的 InstancePath 标识 occurrence，引用文档与编辑宿主分离。
- PersistentSelection 跨 Revision 引用拓扑，Publication 提供稳定接口；显示名、mesh/local ID 只作当前显示证据。
- 产品约束与机构关系分别拥有定义及位姿，复用数值引擎。数据库支持 PostgreSQL/SQLite；人工运行、Agent 与真实集成共用 `.env` 的资源，短期单元对象由测试拥有；逻辑模块不自动成为微服务。

## 按问题进入

局部任务先读最近 [AGENTS](../AGENTS.md)，搜符号和邻近测试；需要领域语义时，每行先读一个命中主题。

| 问题 | 当前事实 | 源码归属 |
|---|---|---|
| 进程、Router、Job、交换 | [运行](architecture/current/runtime.md)、[Jobs](architecture/current/jobs-artifacts.md)、[Worker](architecture/current/worker-contracts.md) | `services/`、`workers/geometry/` |
| Command、参数、CAS、Undo/Redo | [模型历史](architecture/current/model-history.md) | `services/internal/workspace/`、`modelcore/` |
| Sketch、Profile、Body、特征 | [Part/Sketch](architecture/current/part-sketch.md)、[实体 Feature](architecture/current/solid-features.md) | `kernel/occt/`、`workers/geometry/sketch/` |
| 持久身份、几何与显示 | [Naming](architecture/current/persistent-naming.md)、[制品](architecture/current/geometry-representations.md) | `kernel/api/`、`kernel/occt/`、Workspace |
| Product、约束、机构 | [Product](architecture/current/product-assembly.md)、[约束](architecture/current/assembly-constraints.md)、[DMU](architecture/current/kinematics-dmu.md) | Workspace、`kernel/assembly/` |
| 选择、工具、标签、命令目录 | [Web](architecture/current/web.md)、[对象属性/全局选项](architecture/current/object-scene-properties.md)、[编辑上下文](architecture/current/product-edit-context.md)、[公共框架](architecture/current/document-command-framework.md) | `web/apps/cad/src/`、`services/internal/workbenchconfig/` |

## 意图、运行与证据

| 入口 | 回答什么 |
|---|---|
| [当前架构](architecture/current/README.md) | 已实现行为、源码/测试入口、明确限制 |
| [目标设计](architecture/target/README.md) | 长期契约、缺口与候选；不是已交付清单 |
| [后续工作](../plans/README.md) | 未完成工作与退出条件；不自行给候选排优先级 |
| [开发环境](development/README.md)、[Agent 工作流](development/agent-workflow.md) | 配置、资源、构建与渐进验证 |
| [操作指南](guides/README.md)、[参考资料](references/README.md) | 交互步骤 / 外部行为依据 |
| [测试入口](../tests/README.md)、[验证记录](validation/README.md) | 可执行检查 / 实测范围及证据限制 |

启动见[根 README](../README.md)。文档修改读[局部指南](AGENTS.md)和[维护规则](development/documentation.md)。代码、迁移和实际执行决定当前事实；测试存在不等于通过，定向通过不等于工业验收。
