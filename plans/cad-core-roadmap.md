# CAD 核心主线

当前重点：上下文历史插入一致性与同 Body 连续实体特征整体移动。插入按冻结的 Product / Variant / occurrence 校验投影 GeometryKey，正式引用和证据来自独立定义；MOVE_FEATURE 一次提取并移动明确集合，保留内部顺序、抑制项、固定边界、添加/净切除范围及全 Part 校验。当前能力、限制和验证入口见唯一 [Feature 能力表](../docs/architecture/current/solid-features.md#feature-能力表)。新增交互待手工验收，不累计实施日志。

后续按实际建模需求推进：更多合法历史插入/移动；任意深层上下文编辑、跨层更新编排与柔性场景、静态干涉/间隙及交换验收仍未完成。具体边界分别归所属 current 主题，不把设计或接口存在当作交付。

沿用现有命令、稳定引用、权威求值、Naming 和补偿历史；每次完成创建/编辑/修复及必要的真实 Router/Worker、浏览器和热冷重建验证。暂停新增三维 Feature 类型，不扩展 AI/DSL、曲面或新求解框架。已完成或被替代的规划直接删除，历史由 Git 保留。
