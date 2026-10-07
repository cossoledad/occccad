# 知识目录

普通任务从根 [AGENTS](../AGENTS.md) 到最近局部指南，再按符号读取实现和邻近测试。只有跨身份、历史、单位或 Worker 边界才展开主题；无需阅读旧路线或两份完整架构书。

| 问题 | 权威入口 |
|---|---|
| 进程、控制/数据面、Worker、Artifact | [运行](architecture/current/runtime.md)、[Jobs/交换](architecture/current/jobs-artifacts.md)、[几何制品](architecture/current/geometry-representations.md) |
| Workspace、Revision、Command、CAS、幂等、Undo/Redo、参数 | [模型历史](architecture/current/model-history.md) |
| Sketch、Profile、Multi-Body、Feature 求值 | [Part/Sketch](architecture/current/part-sketch.md)、[求值链](architecture/part-feature-evaluation.md)、[实体 Feature](architecture/current/solid-features.md) |
| Naming、Evidence/Lineage、Resolver、Reconnect | [持久命名](architecture/current/persistent-naming.md) |
| STEP/XDE Definition/Occurrence | [交换](architecture/current/jobs-artifacts.md) |
| Product 引用、六族、组、参数/激活、历史发布 | [Product](architecture/current/product-assembly.md)、[当前约束合同](architecture/current/assembly-constraints.md) |
| 装配方程、图编译、QR/SVD、解选择、连续恢复 | [算法](../kernel/assembly/SOLVER_ALGORITHMS.md)、[模块边界](../kernel/assembly/SOLVER_ARCHITECTURE.md) |
| 局部证据、剩余运动与修复 | [Product 分析](architecture/current/product-assembly.md#局部诊断与剩余运动) |
| 选择、显隐、编辑宿主、标签、快照/手柄 | [Web](architecture/current/web.md)、[模型显示](architecture/current/model-display.md)、[编辑上下文](architecture/current/product-edit-context.md)、[树身份](architecture/semantic-tree-interaction.md) |
| 文档适配器、配置目录、Toolbar 与命令注册 | [公共框架](architecture/current/document-command-framework.md) |
| 实时链路 | [Realtime](architecture/current/realtime.md) |
| 验证、观测、证据限制 | [验证](architecture/current/validation.md)、[测试路由](../tests/README.md)、[装配执行器](../tests/assembly-contract/README.md) |
| 运行与环境 | [根 README](../README.md)、[开发环境](development-environment.md)、各可执行单元 README |
| 当前能力和边界总览 | [当前架构](CURRENT_ARCHITECTURE.md) |
| 尚有价值的未实现设计/平台合同 | [设计索引](TARGET_ARCHITECTURE.md) |
| 真实未完成方向 | [后续工作](../plans/README.md) |
| Agent 路由与上下文检查 | [focused TEAA](architecture/agent-efficiency.md) |

每个主题拥有一份合同/算法，不复制完成记录、PASS 数量或旧计划历史。实现语义归 current，未实现设计明确范围；README 负责运行，AGENTS 负责稳定规则。测试存在不等于实际执行，维护者反馈不代替性能/工业验收。

修改导航运行 invoke context-audit：覆盖全仓自有 Markdown 的本地路径与锚点，同时报告默认入口字节规模与大文本热点。稳定机器 ID、schema/policy、命令和 fixture 可以保留旧字面值，不用粗暴零命中规则改写合同。

商业 CAD 参考只在[工作流来源](references/cad-workflows.md)，不是当前能力证据。
