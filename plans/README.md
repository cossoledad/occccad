# 后续工作

当前模型、算法与测试从[知识目录](../docs/README.md)直达；完成计划已退出，历史由 Git 保存。装配主体能力形成当前开发基线；已知细节暂缓，后续按实际需求修复。维护者确认主体功能具备，仍有部分交互、显示问题；不代表工业、并发或性能全部通过。小机构与基础 DMU 已接通首版，等待实机验收；Part Sketch/Feature/Naming 和其他方向按实际需求继续。工程连接继续暂停，无自动恢复日期。

| 方向 | 剩余工作与退出条件 |
|---|---|
| Revolve | 已能生成几何；先补 Face/Edge/Vertex history、cap/seam、轴退化、full/partial、Boolean 后续命名与解析 corpus，再闭合编辑、表达式、预览、面上草图和 Product 引用。 |
| Feature | Hole → Fillet → Chamfer，随后 Pattern/Mirror/Shell/Draft；每项含 schema、参数、精确求值、topology history、引用、UI、Undo/Redo、冷重建与失败。Loft/Sweep 须明确 section/guide/seam 身份。 |
| 关联投影 | trimmed Arc → Face Boundary → Section；[设计](../docs/architecture/target/sketch-projection.md)。当前 unsupported 不用临时近似替代。 |
| 大模型 | 真实容量/瓶颈基线，再补 checkpoint、资源预算、分阶段计算、LOD/chunk、共享 buffers、解码/BVH 与故障验收；[设计](../docs/architecture/target/large-models.md)。续传暂缓，不是前置；基础数据面/XDE 不重做，不宣称完整 AP242 或 1 GiB+/单超大 Body/低复用装配已验收。 |
| 树与依赖 | Feature 贡献的复杂组合与人工验收、子树按需/大树基准、Publication 引用查看/跨 Workspace 并发、更多参数生命周期；按真实场景进入，不默认阻塞装配。 |
| 装配证据与规模 | 缺 Revision 运动证据保持未知，可按需补快照绑定只读刷新。连通性能、浏览器端到端与工业语料专门验收；千行 UI fixture 不替代求解。 |
| 小机构与基础 DMU | 当前已接通单坐标硬驱动、闭环与离散实体检查；[实施、演示与待验收项](kinematics-dmu.md)。下一步先按维护者实机反馈收口，不扩展动力学、多驱、可动嵌套、CCD 或矩阵优化。 |
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
- flexible assembly、DMU 扩展/BOM/PMI：真实场景、来源快照、多精度表示先明确；基础实体 DMU 见当前合同。
- 稀疏/增量求解：真实 connected-component benchmark 证明瓶颈。
- 跨主机、多副本、多用户协作：部署失败、公平性、配额或语义冲突场景触发；现有 S3/实时同步不等于完整协作。
- Configuration/Rule、Surface/3D Wire、Drawing/CAM/CAE：独立用户切片与稳定引用/质量门。

## 完成条件

typed 命令、参数/单位、权威求值、provenance/失败、UI、历史与验证形成闭环。未验收风险不随实现删除；完成事实归主题，执行日志归派生报告。稳定 capability/case/policy/schema/API/fixture 标识保持，不是读者需要学习的路线阶段。
