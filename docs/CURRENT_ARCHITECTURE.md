# 当前实现

参数模型与不可变 Revision 是业务真相；B-Rep、Mesh、Naming、Variant 和分析结果是带 provenance 的可重建制品。Go 控制面/PostgreSQL、C++ Worker 和 React/Three.js 构成可运行垂直切片；Router 当前管理本机 Worker，存储支持 LOCAL/S3。

本页只导航，不复制阶段记录或测试数字。代码、迁移和实际执行决定事实，主题见[知识目录](README.md)。

| 领域 | 当前说明 | 明确边界 |
|---|---|---|
| 进程/数据面 | [运行](architecture/current/runtime.md)、[制品](architecture/current/geometry-representations.md)、[Jobs/XDE](architecture/current/jobs-artifacts.md) | 跨主机调度、完整 AP242、工业容量/渐进工作集未交付或未验收 |
| 历史/参数 | [模型](architecture/current/model-history.md) | semantic rebase、完整协作、Configuration/Rule 尚缺 |
| Sketch/Part | [模型与求值](architecture/current/part-sketch.md) | trimmed Arc 投影、Boundary/Section、完整专业 Sketcher、更多 Feature |
| 命名 | [Naming/Resolver](architecture/current/persistent-naming.md) | Revolve 完整 history 未闭合；导入不是参数化 Feature history，歧义须显式重连 |
| Product/装配 | [Product](architecture/current/product-assembly.md)、[六类合同](architecture/current/assembly-constraints.md)、[算法](../kernel/assembly/SOLVER_ALGORITHMS.md) | 局部分支最优、瞬时 DOF、非线性 UNKNOWN；不保证任意最小冲突集、工业性能或全局最近点 |
| 显示/交互 | [Web](architecture/current/web.md)、[模型显示](architecture/current/model-display.md)、[编辑上下文](architecture/current/product-edit-context.md) | 整体结构树/单进程打开注册表、贡献索引与子树分页、跨 Workspace 并发引用尚有限制 |
| 验证/观测 | [验证](architecture/current/validation.md)、[测试](../tests/README.md) | Node/SSR 不替代浏览器；kernel/RPC 不代表输入至权威显示延迟 |

维护者反馈当前使用场景下拖拽与约束较为稳定；剩余运动呈现尚待本次检查，不扩展为所有工业模型、并发、性能或平台已验收。实际测试范围必须从本次执行报告读取，不复制历史全量通过。

Engineering Connections 明确暂缓，已有 Frame、Contact、Fix Together 和有限运动能力保留。未完成能力与进入条件只在[后续工作](../plans/README.md)维护；有价值的未实现合同见[设计索引](TARGET_ARCHITECTURE.md)。
