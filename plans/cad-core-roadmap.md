# CAD 核心主线

当前重点：维护草图与现有 Feature 基线；添加材料范围中的拔模复制及角度面归属已接通。当前支持、限制及验证入口统一见 [Feature 能力表](../docs/architecture/current/solid-features.md#feature-能力表)、[Part/Sketch](../docs/architecture/current/part-sketch.md)与[工作台规范](../docs/guides/workbench.md)。不在本文件累计实施日志或完成清单。

后续按实际建模需求推进：其余关联终止、贯穿 Hole 工具、切除范围、更多合法历史插入/移动；装配嵌套更新与柔性场景、静态干涉/间隙及交换验收仍未完成。具体边界分别归所属 current 主题，不把设计或接口存在当作交付。

沿用现有命令、稳定引用、权威求值、Naming 和补偿历史；每次完成创建/编辑/修复及必要的真实 Router/Worker、浏览器和热冷重建验证。暂停新增三维 Feature 类型，不扩展 AI/DSL、曲面或新求解框架。已完成或被替代的规划直接删除，历史由 Git 保留。
