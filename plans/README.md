# 后续工作

当前模型、算法与测试从[知识目录](../docs/README.md)直达；完成计划已退出，历史由 Git 保存。当前使用反馈为拖拽与约束较为稳定，剩余运动呈现仍需本次人工检查；不代表工业、并发或性能全部通过。新反馈先做定向闭环，后续主线转向 Part Sketch/Feature/Naming。工程连接明确暂缓，无自动恢复日期。

| 方向 | 剩余工作与退出条件 |
|---|---|
| Revolve | 已能生成几何；先补 Face/Edge/Vertex history、cap/seam、轴退化、full/partial、Boolean 后续命名与解析 corpus，再闭合编辑、表达式、预览、面上草图和 Product 引用。 |
| Feature | Hole → Fillet → Chamfer，随后 Pattern/Mirror/Shell/Draft；每项含 schema、参数、精确求值、topology history、引用、UI、Undo/Redo、冷重建与失败。Loft/Sweep 须明确 section/guide/seam 身份。 |
| 关联投影 | trimmed Arc → Face Boundary → Section；[设计](../docs/architecture/target/sketch-projection.md)。当前 unsupported 不用临时近似替代。 |
| 大模型 | 真实容量/瓶颈基线，再补 checkpoint、资源预算、分阶段计算、LOD/chunk、共享 buffers、解码/BVH 与故障验收；[设计](../docs/architecture/target/large-models.md)。续传暂缓，不是前置；基础数据面/XDE 不重做，不宣称完整 AP242 或 1 GiB+/单超大 Body/低复用装配已验收。 |
| 树与依赖 | Feature 贡献索引、子树按需/大树基准、Publication 引用查看/跨 Workspace 并发、更多参数生命周期；按真实场景进入，不默认阻塞装配。 |
| 装配证据与规模 | 缺 Revision 运动证据保持未知，可按需补快照绑定只读刷新。连通性能、浏览器端到端与工业语料专门验收；千行 UI fixture 不替代求解。 |
| 维护 | 按重复修改/漏检证据优化职责和检查路由，保持行为等价，不按文件数/行数重构。 |

## 候选进入条件

- Engineering Connections、Kinematics/动力学：明确 frame/limits/运动状态合同后决定；瞬时 DOF 不是关节。
- flexible assembly、DMU/BOM/PMI：真实场景、来源快照、多精度表示先明确。
- 稀疏/增量求解：真实 connected-component benchmark 证明瓶颈。
- 跨主机、多副本、多用户协作：部署失败、公平性、配额或语义冲突场景触发；现有 S3/实时同步不等于完整协作。
- Configuration/Rule、Surface/3D Wire、Drawing/CAM/CAE：独立用户切片与稳定引用/质量门。

## 完成条件

typed 命令、参数/单位、权威求值、provenance/失败、UI、历史与验证形成闭环。未验收风险不随实现删除；完成事实归主题，执行日志归派生报告。稳定 capability/case/policy/schema/API/fixture 标识保持，不是读者需要学习的路线阶段。
