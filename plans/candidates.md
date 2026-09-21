# 候选方向与进入条件

> 候选，不是已排期承诺。返回[统一路线](README.md)。先满足进入条件，再建立具备用户场景、范围、依赖和验收的计划。

| 方向 | 进入条件 | 范围与明确边界 |
|---|---|---|
| 参数产品化 | Product 更新/Release 基线稳定，有真实配置用例 | Configuration、Design Table、Rule/Check、批量研究；已有 Release gate 不等于这些能力已交付 |
| Surface / 3D Wire | 稳定 curve/section/guide identity，branch/seam/UV/pcurve 与质量门明确 | 基础线框/曲面 → intersection/trim/join → sweep/loft → fill/offset/实体桥接 → styling；目标见[曲面模型](../docs/architecture/target/surface-model.md)；Sketch 二维 snapshot 不充当三维真相 |
| DMU 与替换管理 | 不可变产品快照及多精度表示、代表性产品 corpus | BOM/effectivity、Replace、flexible subassembly、clash/clearance、section、persistent issue；不把当前 rigid Product 当 flexible |
| Kinematics | Engineering Connection、joint frame、limits 与运动状态有完整合同 | Mechanism、FK/IK、driver/law、trace、swept envelope；静态 Angle winding 不代替时间状态 |
| 大规模求解（M7） | 代表性 connected-component benchmark 证明瓶颈 | 再决定 block-sparse、增量 factorization、backend 或独立 Assembly Worker |
| 共享制品与长求值 | 首个多副本/跨主机部署，或同步求值超时有证据 | ArtifactStore 共享后端、冷重建、durable evaluation、取消/重试与结果 gate；不强制同时更换 PostgreSQL Jobs |
| 跨主机计算 | 制品可共享，节点失效/公平性/资源需求明确 | Worker registry/lease、调度、配额、backpressure；NATS/Kubernetes 为评估候选 |
| 协作与大装配显示 | 多用户冲突或表示加载基准明确 | semantic rebase、presence/preview、LOD/渐进加载；已有提交同步不能替代协作冲突语义 |
| 工程工作台生态 | 有独立用户切片及稳定扩展合同 | Drawing/PMI、Sheet Metal、CAM/CAE、电气、插件；按[扩展边界](../docs/architecture/target/quality-extensions.md)逐域设计 |

平台支线不阻塞 Part/Assembly 正确性；一旦实际部署提出强制条件，则该部署必须等条件达成。曲面、DMU、仿真也不应仅因编号靠后而强制串成一条依赖链。
