# 当前架构

这里记录代码、迁移和测试支持的当前行为。参数模型与不可变 Revision 是业务真相，几何和分析是可重建制品。主题正文拥有详细合同，本页只导航；项目意图与全局调用链见[项目地图](../../README.md)。

| 领域 | 主题 | 当前边界 |
|---|---|---|
| 进程与计算 | [运行](runtime.md)、[Worker 合同](worker-contracts.md)、[Router/Jobs/交换](jobs-artifacts.md) | 本机 Router；跨主机调度与完整 AP242 未交付 |
| 模型与历史 | [命令/参数/历史](model-history.md) | CAS、幂等、补偿历史；semantic rebase、Configuration/Rule、完整协作仍缺 |
| Part 与 Sketch | [Part/Sketch](part-sketch.md)、[求值链](part-feature-evaluation.md)、[实体 Feature](solid-features.md)、[抽壳算法](shell-algorithm.md) | 多 Body、旋转、布尔、局部修改、阵列/镜像和基础放样已有实现；各特征范围与失败门以主题为准 |
| 命名与显示制品 | [Naming/Resolver](persistent-naming.md)、[几何表示](geometry-representations.md)、[Visual/Naming 格式](visual-naming-artifacts.md)、[颜色材质](surface-appearance.md) | 绑定按 Revision/Body；导入冻结身份不等于参数化来源历史，歧义须显式重连 |
| 产品与装配 | [Product](product-assembly.md)、[六类约束](assembly-constraints.md)、[数值算法](../../../kernel/assembly/SOLVER_ALGORITHMS.md)、[模块边界](../../../kernel/assembly/SOLVER_ARCHITECTURE.md) | 局部分支、瞬时 DOF、非线性 UNKNOWN；不保证全局最近点或任意最小冲突集 |
| 机构与 DMU | [机构、试动、录制、回放](kinematics-dmu.md) | Ground/旋转/棱形/圆柱/平面/刚性/球面接合、独立坐标命令；动力学与连续碰撞未交付 |
| 工作台 | [Web](web.md)、[对象属性/全局选项](object-scene-properties.md)、[编辑会话](product-edit-context.md)、[语义树](semantic-tree.md)、[模型显示](model-display.md)、[文档/命令框架](document-command-framework.md) | 全局观看设置与对象编辑分离；大树分页、复杂贡献与跨 Workspace 并发仍有限制 |
| 实时与验证 | [Realtime](realtime.md)、[可观测性](validation.md)、[测试路由](../../../tests/README.md) | Node/Mock 不替代真实浏览器；kernel/RPC 计时不等于端到端延迟 |
| 自动化与 AI 建模 | [操作、查询、任务与显示](automation.md) | 持久 Run、分段 Part、冻结选择与普通轴孔装配；MCP/CLI、大树索引及复杂 ContextVariant 待实施 |

更多实体特征、trimmed Arc/Boundary/Section、规模与验收缺口见[后续工作](../../../plans/README.md)；未实现合同和候选见[目标设计](../target/README.md)。实测结论归[验证记录](../../validation/README.md)，不能复制成当前全量通过声明。
