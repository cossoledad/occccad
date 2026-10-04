# 有限面域圆角验证

## 输入与根因

目标 STEP 只有最终形体，没有原始圆角前的精确形体、选边及特征历史。以下是几何族最小复现，不能宣称已还原两个文件的原始建模操作。

- Solar 类型：60 × 30 × 2.2 mm 薄板，四条底边 R3。原 OCCT 构造失败，四个 contour 均为 `ChFiDS_StartsolFailure`。底面与侧面的相切圆在无限支撑上存在，但侧面接触点越过顶端有限边界。
- Fan 类型：2 mm 厚、30° 倾角叶根，凹平面连接 R15，加入倾斜轴向与上下限制平面。以解析圆柱扇区和独立圆弧截面比较。另一项全局限制面消耗全部 R15 过渡的输入须拒绝，不能把填满后的平面实体视为圆角成功。

## 当前构造与边界

优先保留原有效通用圆角。新增受控的解析工作支撑延伸、恒半径过渡、限制面裁剪及真实 Boolean/Fillet/同域合并历史。原模型尺寸不变；延伸仅发生在内核私有工作形体中。前端、Quantity、参数、预览提交、失败状态与 Undo/Redo 沿用既有链路。

- 凸等截面平面/圆柱侧壁与平行端面；凸全平面体的有限支撑越界。带预先存在的小半径圆柱角侧壁的薄板虽然能生成闭合的 R3/四个 B-spline 候选，但边界法向检查失败，当前明确拒绝，不能作为已支持能力。
- 单条凹平面根部，端点必须由全局材料限制平面截断。
- 不覆盖多个凹根交会、任意曲面、自动构造任意 G1 角区，也不推断 STEP 中不存在的特征意图。

## 验证入口

- `kernel/occt/tests/fillet_boundary_scenarios.cpp`：原始失败、目标曲面测量、半径跨越厚度、2/2.2/2.3/3/4/3 往返、刚体变换、反向选边、冷内核重算、精确差集、半径、自干涉、pcurve、限制范围、上游不可变与 Naming。
- `services/internal/geometry/fillet_boundary_worker_integration_test.go`：真实 Worker、阶段语义选边、R3 往返、完整 Naming 与 BREP/GLB。
- `services/internal/control/fillet_boundary_integration_test.go`：独立临时 SQLite/ArtifactStore、真实 Router/Worker；预览不改 Head、参数重算、Undo/Redo、厚度更新、重新读取。
- 回归 `GeometryExchange.ChamferedTrihedralCornerFillets`、`GeometryExchange.ChamferedBoxBottomFilletsAndInwardShell`、`TestCppWorkerChamferedCornerFillets`。

## 目标差异与人工验收

Solar 四个 R3 侧面与原最终厚度可重建。仅选直角矩形四条底边时，OCCT 角区为圆柱相交边界；目标 STEP 有四个 B-spline 角区，不能称为几何等效。包含 R0.5/R1 小圆柱侧壁的源形体生成的角区，与相邻 R3 圆柱存在约 4°–7° 的法向跳变。当前返回 `FILLET_BOUNDARY_CORNER_TANGENCY_FAILED`，没有采用该候选。Solar 的光顺角区仍未修复，需要独立的相切角区构造；不能继续以原生角补片的 IsDone 作为完成依据。应分别观察侧面和四角，不能仅比包围盒或体积。

Main Fan 的定向用例验证倾斜叶根 R15 的圆柱、切向连接和限制面裁剪；尚未证明完整风扇（含实际轮毂、双叶根、孔槽及后续操作）与 STEP 等效。

人工验收：薄板底边圆角输入 R2→R2.2→R3→R4→R3，检查厚度及上边界；改厚度、撤销/重做并重开。叶根输入 R15，检查保留的圆柱面、支撑接触和上下终止边界；对过渡面被完全消耗的输入检查明确失败。最后分别比较两个目标的完整角区与叶根，不以最小复现通过代替最终装配模型验收。

## 实测与剩余限制

最终定向验证：7 项 C++ CTest 通过（56.50 s），3 项真实 Worker/Router Go 集成通过；`invoke context-audit`、`git diff --check` 通过。角区回归通过表示正确拒绝非相切候选，不表示 Solar 角区已修复。

本机 Debug 定向运行中，原失败的 R3 薄板通用构造约 40 ms，R15 叶根约 23 ms；解析延伸/裁剪的薄板与叶根构造通常约 0.3–0.6 s。圆柱侧壁端部 R3 的环面重建约 0.3 s。角区失败候选的构造约 0.1 s、限制面裁剪约 1.4–1.8 s；法向检查拒绝后不进入正式求值结果。以上是最小用例的实测范围，并非目标整件耗时或加速比。

圆角的首尾生成关系显式跨越隔离副本：OCCT 同时删除中间边并生成面时，不能因历史合成丢失真实 Generated 关系。未通过相切检查的候选不产生可采用制品。

完整 Solar Panel / Main Fan 的几何等效尚未完成。尤其多个凹根、已有曲面参与的复杂叶根邻域不在当前单条全平面凹根分支范围内。不运行浏览器自动测试、全量单测，不清理用户数据。
