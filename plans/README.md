# 后续工作

当前模型、算法与测试从[知识目录](../docs/README.md)直达；完成计划已退出，历史由 Git 保存。装配主体能力形成当前开发基线；已知细节暂缓，后续按实际需求修复。维护者确认主体功能具备，仍有部分交互、显示问题；不代表工业、并发或性能全部通过。机构基础范围已贯通统一约束试动、双面板关键帧录制、有限路径回放和独立产品转换，当前合同归当前架构，后续路径质量和扩展语义归目标架构；Part Sketch/Feature/Naming 和其他方向按实际需求继续。工程连接继续暂停，无自动恢复日期。

| 方向 | 剩余工作与退出条件 |
|---|---|
| 实体 Feature 范围与验收 | 当前旋转 Naming、布尔、圆角/倒角、拔模、抽壳、阵列/镜像和基础放样已实现，事实归[实体 Feature](../docs/architecture/current/solid-features.md)。剩余多工具交集、可变半径、非等距倒角、复杂拔模、带导轨/孔/任意不同边数放样按需求进入；完整鼠标建模链、复杂实体和参数组合仍需独立浏览器证据。 |
| 新特征候选 | 独立 Hole、Sweep 等先明确用户切片与稳定 section/guide/seam 身份，再贯通参数、权威求值、Naming、预览、历史及冷重建；不把现有切除孔当作独立 Hole DSL，不在本表预设实施顺序。 |
| 关联投影 | trimmed Arc → Face Boundary → Section；[设计](../docs/architecture/target/sketch-projection.md)。当前 unsupported 不用临时近似替代。 |
| 大模型 | 真实容量/瓶颈基线，再补 checkpoint、资源预算、分阶段计算、LOD/chunk、共享 buffers、解码/BVH 与故障验收；[设计](../docs/architecture/target/large-models.md)。续传暂缓，不是前置；基础数据面/XDE 不重做，不宣称完整 AP242 或 1 GiB+/单超大 Body/低复用装配已验收。 |
| AI 原生建模 | DeepSeek 对话与 Part 执行见[当前合同](../docs/architecture/current/automation.md)；按[后续计划](ai-native-modeling.md)补线上质量基线、大树索引/集合、MCP/CLI 与复杂装配。 |
| 树与依赖 | Feature 贡献的复杂组合与人工验收、子树按需/大树基准、Publication 引用查看/跨 Workspace 并发、更多参数生命周期；按真实场景进入，不默认阻塞装配。 |
| 装配证据与规模 | 缺 Revision 运动证据保持未知，可按需补快照绑定只读刷新。连通性能、浏览器端到端与工业语料专门验收；千行 UI fixture 不替代求解。 |
| 机构路径质量与扩展 | 基础试动／录制实现见[当前合同](../docs/architecture/current/kinematics-dmu.md)。后续按[统一设计](../docs/architecture/target/kinematics-dmu.md#记录路径的求解回放与结果)扩展独立段质量、局部制品重算和适应显示误差的采样；补充目标验收矩阵中尚未执行的场景。继续复用同一约束会话、稳定零位与合格检查点；动力学、多机构同步、可动嵌套和连续碰撞按明确需求另定范围。 |
| 维护 | 按重复修改/漏检证据优化职责和检查路由，保持行为等价，不按文件数/行数重构。 |

## 装配暂缓问题

| 现象 | 影响与复现信息 | 暂缓原因 |
|---|---|---|
| 交互、显示仍有明显细节问题 | 维护者已反馈，但未提供具体操作、对象或截图；本次未建立可复现案例，不推断根因或声称已解决。 | 主体能力已形成基线，停止持续扩展；按实际需求和复现证据修复。 |
| 部分 Revision 的剩余运动显示待计算 | 当前版本缺求值结果、结果仅覆盖受影响范围或 component 未 solved 时保留未知；没有独立只读刷新入口。 | 如实呈现证据缺口，不为填表触发模型修改或扩大求解范围。 |
| 硬约束可行但运动偏好未收敛 | 已保留并精确回放真实请求；原预算失败，增加预算或改善初始猜测后成功。见[问题说明](assembly-preference-nonconvergence.md)。 | 与性能优化分开跟踪；先记录证据，后续详细设计通用收敛改进与有界恢复。 |

以上不阻止当前基线收口。浏览器视觉、端到端性能及工业容量仍属未完整验收边界，不计为已解决问题。

## 候选进入条件

- Engineering Connections、机构高级关节/动力学：按真实需求明确映射、耦合、动力学状态合同；瞬时 DOF 不是关节。
- flexible assembly、DMU 分析扩展/BOM/PMI：真实场景、来源快照、多精度表示先明确；机构之外的分析不冒充当前应用能力。
- 稀疏/增量求解：真实 connected-component benchmark 证明瓶颈。
- 跨主机、多副本、多用户协作：部署失败、公平性、配额或语义冲突场景触发；现有 S3/实时同步不等于完整协作。
- Configuration/Rule、Surface/3D Wire、Drawing/CAM/CAE：独立用户切片与稳定引用/质量门。

## 完成条件

typed 命令、参数/单位、权威求值、provenance/失败、UI、历史与验证形成闭环。未验收风险不随实现删除；完成事实归主题，执行日志归派生报告。稳定 capability/case/policy/schema/API/fixture 标识保持，不是读者需要学习的路线阶段。
