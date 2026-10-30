# CAD 核心主线

当前重点回到装配关联与静态干涉／间隙：按现有公开接口、接受更新、定位与修复机制推进，首轮范围和验收方案按实际任务确定，不另建更新或求解体系。

历史创建阶段性收口：独立 Part 与直接、可编辑、READY 的 Product ContextVariant 支持历史补草图及同 Body、单方向 FINITE 的 LINEAR_EXTRUDE ADD/REMOVE；沿用四种修饰插入、受约束移动、完整候选检查及历史。能力和边界见唯一 [Feature 能力表](../docs/architecture/current/solid-features.md#feature-能力表)，新增上下文交互仍待手工验收。

保留后续扩展：其他生成器的合法历史创建、更多受约束移动、任意深层上下文编辑、跨层更新编排、柔性场景及交换验收；具体限制归所属 current 主题，本阶段不继续扩展。


沿用现有命令、稳定引用、权威求值、Naming 和补偿历史；每次完成创建/编辑/修复及必要的真实 Router/Worker、浏览器和热冷重建验证。暂停新增三维 Feature 类型，不扩展 AI/DSL、曲面或新求解框架。已完成或被替代的规划直接删除，历史由 Git 保留。
