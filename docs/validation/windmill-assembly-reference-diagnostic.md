# 风车装配：圆柱同轴与诊断

文档 `01a10c50-e0c9-7daa-beb7-68fb9ed2ef4e`，Part6/Part7 的圆柱支持约束出现 `ASSEMBLY_PREFERENCE_NOT_CONVERGED`。原始记录 `20261005T173335.573535944Z-98414999a8a68663` 几何已收敛，但 reference 旋转约 10⁻⁶，reference optimality≈5.21×10⁻⁷、total optimality≈0.01209，超过 10⁻⁸ 门槛。

根因：第二级最小总运动仅用 reference 标量能量上界，允许花掉 10⁻¹² 容差，偏离第一级零运动最优；非线性几何恢复没有同步保持 reference。修复：零 reference 最优时同步恢复其残差，非零最优仍用曲率最优子空间，保持硬约束及严格接纳检查。没有把 Stalled 当成功。

原文件 263796 字节主要来自几何 ID 内嵌引用 JSON，在描述和约束支持处重复。最小数值 fixture 约 10 KB；业务来源留在 CAD_DIAGNOSTIC 的不可变上下文中，重放只用纯值描述、位姿、约束、意图、分支和容差。恢复文件时从上述原始记录重新提取完整数值输入，仅缩短运行时几何 ID 并设置独立重放 requestId。

人工验收：刷新装配，对上述两个圆柱重新计算/编辑同轴约束；确认 Verified、reference 不额外漂移，再 Undo/Redo、重开。NotUpdated 的其他诊断只需复制 `CAD_DIAGNOSTIC 文档/记录编号 错误码`；预览与属性面板均提供入口。失败时 DOF 描述的是已接纳集合，原失败试算被隔离，不应读取其位姿为正式结果。

上轮已做：原始 C++ 回归、装配内核 205 场景及 21 corpus、Go 诊断/隔离/最小重放单测、真实 Router/Worker 重放、装配历史及失败诊断回归、性能基线；未运行浏览器自动化或无差别全量单测。原文档同一支持通过真实服务的只读预览：Verified、两侧 Connected；reference optimality≈7.52×10⁻¹¹、total optimality≈1.26×10⁻¹⁴，Head 不变且预览已丢弃。实机交互仍由人工验收。

诊断记录包含失败试算数值重放及 SolveManifest digest；缓存命中时可用 digest 查询冻结输入，不依赖最新 Head。已知限制：额外历史回归的 Space Fix/刚性簇冲突在修改前代码和原 Worker 同样失败，留作独立问题；两个依赖专用 PostgreSQL/corpus 的编辑意图测试被跳过，未计为通过。
