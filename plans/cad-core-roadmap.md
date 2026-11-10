# CAD 核心主线

唯一 CAD 主线任务入口，只保留待推进事项。草图、实体生成器、成员重执行及输入修复的现有范围见[整体设计流程](../docs/architecture/current/part-design-workflow.md)和[实体能力表](../docs/architecture/current/solid-features.md#feature-能力表)；已完成清单与实施过程由 Git 保留。

## 待推进事项

| 方向 | 范围与验收重点 | 当前依据 |
|---|---|---|
| 工程曲面扩展 | A–D 的独立 Part 有界能力：关联曲线、工程曲面编辑/实体化/交换；姿态/有界边界导向、spine、显式点对应、Boundary/Blend、逐边多项式支撑 G1/受限 G2、只读曲率分析。后续按 current 限制推进通用导向/支撑、一般曲面编辑及测量/发布/Product 互用 | [当前边界](../docs/architecture/current/wireframe-surfaces.md)、[阶段设计](wireframe-surface-design.md)、[商业 CAD 对照](../docs/references/surface-modeling.md) |
| 几何测量交互 | 依据实机反馈完善专用上下文来源、标签排布和选择语义；保持临时计算、保留/更新及标注历史边界 | [几何测量](../docs/architecture/current/geometric-measurement.md) |
| 零件持续建模性能 | 在语义索引/共享检查点、未变面网格复用、无损传输和批量高亮基础上，扩大复杂 STEP/长特征链样本；继续削减首次完整剖分与精确布尔成本，建立进程整体内存控制，并覆盖尚不能中断的积分/构造器；首个有效操作与修改参数分别测量 | [Part 求值](../docs/architecture/current/part-feature-evaluation.md#零件增量求值)、[测量边界](../docs/validation/part-feature-performance.md) |
| 空间分析与干涉性能 | 优化持续变化构型的精确交集成本及大装配规模；分别验证复杂实体、显示精度、失效、取消与只读边界 | [空间分析](../docs/architecture/current/product-assembly.md#空间分析对象与手动更新) |
| 历史与上下文扩展 | 跨 Body 历史创建、更多受约束移动、深层上下文编辑及跨层更新编排；逐项明确合法来源、默认构型与 occurrence 一致性 | [实体 Feature](../docs/architecture/current/solid-features.md)、[编辑上下文](../docs/architecture/current/product-edit-context.md) |
| 骨架与跨零件引用 | 从消费参数或几何输入建立引用，在所属对象内编辑/修复；新引用编辑器、带链接复制及深层编排仍待实施，复用内部 UpdatePlan | [流程设计](skeleton-reference-workflow.md) |
| 柔性场景与交换验收 | 单独定义适用模型、数据合同与真实交换/重建检验，再纳入当前能力 | [Product](../docs/architecture/current/product-assembly.md) |

全局自动/冻结更新及连续防穿透尚未实现；机构试动现有[离散采样提示/停止](../docs/architecture/current/kinematics-dmu.md#试动和录制会话)不能替代连续检测。任意自由曲面、放样导向及局部特征范围重执行等限制仍由各 current 主题维护，不因计划精简视为已解决。

## 每项完成条件

沿用稳定 ID、PersistentSelection、Publication、InstancePath、权威求值、完整 Naming、Command/ChangeSet/CAS 与补偿历史。候选冻结输入与上下文；取消、过期响应、失败诊断、单位和预算不得因优化而放宽。

先建立针对性检验，再实现创建/编辑/修复链路；覆盖参数/公式、预览不写 Head、一次提交、未知回执幂等、上游更新、失效修复、Undo/Redo、重开和冷重建。实体/协议经过真实 Router/Worker，交互另做浏览器验收；性能结论声明输入、环境及计时边界。

结果并入所属 current，操作归 guides，必要测量归 validation，原始报告归 `build/agent-check/`。运行[渐进验证](../docs/development/agent-workflow.md)和文档/合同检查；未执行与 skip 不算通过。不顺带扩展无关 AI/DSL 或新求解框架。
