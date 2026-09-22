# occccad 目标架构

> 2026-09-21 整理。本文及其分册定义长期语义，不代表全部已实现。当前行为见[当前架构](CURRENT_ARCHITECTURE.md)，下一步工作见[统一路线](../plans/README.md)。

## 阅读规则

### 0.2 信息权威顺序

发生冲突时按问题类型判断，而不是机械规定单一文件永远优先：

| 问题 | 权威来源 | 处理方式 |
|---|---|---|
| 当前系统实际做什么 | 代码、迁移、测试、`CURRENT_ARCHITECTURE.md` | 文档与代码不符时先验证运行事实，再修正文档或缺陷 |
| 长期模型和边界应该是什么 | 本文档的平台不变量与目标契约 | 实现偏离前必须形成明确架构判断并更新本文档 |
| 某个可执行单元如何运行 | 对应服务 README、构建配置 | 接口、环境变量或故障语义变化时同步更新 README |
| 某个外部库当前版本/能力 | 锁文件、官方文档、验证报告 | 本文中的候选与版本可能过期，引入前必须重新核实 |
| 某次交付的具体范围 | Issue/计划/变更说明 | 不得用短期范围反向削弱长期数据兼容性和平台不变量 |

若现有实现证明目标设计不可行、过度复杂或已有更优方案，应以证据推动修订，而不是为了“符合文档”继续堆叠错误抽象。修订必须说明被替代假设、影响范围、迁移方式和验证证据。

### 0.3 规范强度

本文使用以下强度理解设计表述：

- **平台不变量 / 必须 / 禁止**：跨模块长期约束，除非完成架构变更评审与迁移设计，否则不得偏离；
- **目标契约 / 应**：默认实现方向；局部替代方案必须证明语义等价、复杂度更低或验证结果更好；
- **推荐 / 建议**：当前最合理的工程默认值，可基于实测数据调整；
- **候选 / 可评估**：技术研讨结论，不代表已经选型、引入依赖或承诺兼容；
- **路线阶段**：交付顺序与成熟度门，不表示前一阶段要实现最终全部功能。

文中的 Proto、类型和目录通常是契约草案，用来固定语义和边界，不要求逐字复制字段名。实现可以采用更简洁的数据结构，但稳定身份、单位、版本、错误、幂等和迁移语义不能丢失。

## 分层目录

### 平台与模型核心

- [愿景、平台原则与逻辑边界](architecture/target/system.md)
- [命令、事务与历史](architecture/target/commands-history.md)
- [参数、依赖与求值](architecture/target/parameters-evaluation.md)
- [模型接口、存储与验证](architecture/target/model-governance.md)
- [计算模块与进程边界](architecture/target/compute-boundaries.md)
### Sketch 与 Part

- [Sketch 模型与实体表示](architecture/target/sketch-model.md)
- [Sketch 实体编辑、血缘与查询](architecture/target/sketch-editing.md)
- [Sketch 约束、求解与 Profile](architecture/target/sketch-solver.md)
- [Sketch 持久化、交互与验证](architecture/target/sketch-integration.md)
- [Part、Body 与实体生成](architecture/target/part-model.md)
- [实体特征扩展契约](architecture/target/part-extensions.md)
- [Feature 求值、命名与质量门](architecture/target/part-evaluation.md)
### Surface / 3D Wire（候选）

- [曲面与三维线框模型](architecture/target/surface-model.md)
- [曲面 Feature 与实体桥接](architecture/target/surface-features.md)
- [曲面质量、连续性与命名](architecture/target/surface-quality.md)
- [曲面计算、交互与验证](architecture/target/surface-integration.md)
### Product、Assembly 与工程扩展

- [Product 结构、Publication 与上下文](architecture/target/product-context.md)
- [六类约束、自由度组合与激活合同](architecture/target/assembly-constraints.md)
- [装配约束、求解与交互](architecture/target/assembly.md)
- [运动学、DMU 与仿真候选](architecture/target/kinematics-dmu.md)
- [Product 求值、资源与验证](architecture/target/product-evaluation.md)
- [持久拓扑命名](architecture/target/persistent-naming.md)
### 分布式平台与交付治理

- [调度、通信、存储与制品](architecture/target/distributed-platform.md)
- [大文件导入、根拓扑身份与大模型工作集（设计提案）](architecture/target/large-models.md)
- [安全、可观测性与扩展边界](architecture/target/quality-extensions.md)
- [决策触发、验证与交付](architecture/target/delivery-governance.md)

## 设计收敛

- 参数模型、不可变 Revision、typed stable identity 与可重建制品保持为共同基础；产品功能不以跨主机部署为前置。
- 逻辑模块不自动对应微服务。Assembly 是独立算法库，当前由 Geometry Worker 承载；独立 Worker、消息总线和跨主机 Scheduler 由负载或隔离证据触发。
- Product 内上下文使用 Part ContextInput + Product ContextBinding + 派生 Variant。独立 Part 的受控外部引用有独立使用范围，不能再给普通产品编辑增加并行入口。
- 目标分册只保存契约、算法边界与质量门；删除过时的 C/S/F/A/P 实施时间表、旧矩形/PAD/Product 迁移提案及重复现状。交付依赖只在统一路线维护。
- 当前尚未发布，不为实验数据维护永久 adapter/双写。已有 UI transport adapter 不代表历史兼容承诺；正式发布后的旧 Revision 可读性与追加迁移仍是长期要求。
- 分册保留旧语义章节号便于检索，不再作为开发计划编号。文中局部 P0/P1/P2 表示该能力的基础/扩展/高级范围；它们不是全局先后关系。M3–M7 仅表示求解器成熟度门。
- 目录草案、Proto 示例、开源候选和性能目标都不是当前 API/版本承诺。引入或升级时查锁文件、官方资料和真实 corpus，不能从设计文本推断已安装依赖。

## 维护

每个主题只维护一份权威契约，当前事实进入 `architecture/current/`，仍有效的目标进入 `architecture/target/`。完成计划后转入事实分册并删除已完成条目；未通过的验收保留在计划中。对稳定身份、历史、单位、拓扑引用或跨文档一致性的实质变更仍需先澄清，目录整理不授权改变这些语义。
