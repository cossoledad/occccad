# CAD 核心主线

当前重点：单向贯穿 Hole 的阵列与镜像。三孔型复用 GENERATOR_TOOL，按成员及冻结的阵列前目标重建主孔范围，保留入口尺寸、单向性和稳定复制链，接通多种子、二次复制、阶段检查点及下游修复；TO_PLANE 工具复制仍关闭。当前能力和限制见唯一 [Feature 能力表](../docs/architecture/current/solid-features.md#feature-能力表)，关联输入维护见[发布与关联设计](../docs/architecture/current/product-assembly.md#发布与关联设计)。不累计实施日志。

后续按实际建模需求推进：切除材料范围、更多合法历史插入/移动；任意深层上下文编辑、跨层更新编排与柔性场景、静态干涉/间隙及交换验收仍未完成。具体边界分别归所属 current 主题，不把设计或接口存在当作交付。

沿用现有命令、稳定引用、权威求值、Naming 和补偿历史；每次完成创建/编辑/修复及必要的真实 Router/Worker、浏览器和热冷重建验证。暂停新增三维 Feature 类型，不扩展 AI/DSL、曲面或新求解框架。已完成或被替代的规划直接删除，历史由 Git 保留。
