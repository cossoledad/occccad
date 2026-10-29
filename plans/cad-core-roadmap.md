# CAD 核心主线

当前重点：基于公开接口的单个直接来源组件替换。已接通 Part→Part、Product→Product、实际使用接口对应、单实例候选预览与同一 ChangeSet 的关联/约束/转发接口迁移；接收端本地输入、深层/批量替换、原始拓扑匹配与机构迁移仍不开放。继续巩固模块接口分层接受及孔位编辑修复。当前能力及边界见[发布与关联设计](../docs/architecture/current/product-assembly.md#发布与关联设计)，草图与 Feature 的唯一能力表见 [Feature 能力表](../docs/architecture/current/solid-features.md#feature-能力表)。不累计实施日志。

后续按实际建模需求推进：其余关联终止、贯穿 Hole 工具、切除范围、更多合法历史插入/移动；任意深层上下文编辑、跨层更新编排与柔性场景、静态干涉/间隙及交换验收仍未完成。具体边界分别归所属 current 主题，不把设计或接口存在当作交付。

沿用现有命令、稳定引用、权威求值、Naming 和补偿历史；每次完成创建/编辑/修复及必要的真实 Router/Worker、浏览器和热冷重建验证。暂停新增三维 Feature 类型，不扩展 AI/DSL、曲面或新求解框架。已完成或被替代的规划直接删除，历史由 Git 保留。
