# 基础颜色材质

基础外观是 Revision 中的业务数据。当前支持不透明 sRGB 纯色、STEP 整体色和分面色，以及导入装配中已有的实例颜色覆盖；没有图片纹理、透明度、材质库或可编辑的 PBR 参数。

## 所有权与编辑

- `PartBody.appearance` 保存整体 `color` 和 `faces[].anchor + color`；anchor 使用已有 `SemanticTopologyRef`。颜色为规范小写 `#rrggbb`，黑色不是缺省值。
- Body 树菜单“颜色材质”经 CommandRegistry 打开编辑表单。`SET_BODY_COLOR` 适配到 `occccad://part/body/set-color`，写 `body.appearance` PropertySlot，沿用幂等事务、CAS、ChangeSet 和两步 Undo/Redo。设置整体色替换当前分面赋色，使用默认颜色清除完整外观；撤销可恢复导入分面色。
- 外观进入模型摘要及独立 `appearance:<BodyId>` 依赖节点，不进入实体几何输入。颜色编辑和补偿复用冻结的 Body 几何，不运行 Sketch/OCCT 求值。属性检查和文件面板保持只读。
- Product 的 `instances[].color` 保留导入文件已有的 occurrence 整体覆盖。当前没有实例颜色编辑入口；覆盖随实例插入的正式历史保存。子实例有显式颜色时优先于祖先颜色，最终覆盖该 occurrence 的 Body 和分面色；未覆盖时使用其已接受零件 Revision 的外观。

## 导入与导出

STEP 通过 XDE ColorTool 读取整体、子形体、面与组件颜色。Inspect 输出颜色以及冻结的 definition-local BREP；局部面 ID 只在这次交换快照中有效。Jobs 将分面颜色绑定到同一 BREP SHA-256 下已分配的导入拓扑稳定身份，正式模型不保存面数组位置或 OCCT local ID。

分面颜色在需要拓扑修复且无法证明对应关系时以 `EXCHANGE_COLORED_FACE_REPAIR_UNSUPPORTED` 拒绝，不按修复后的面顺序套色。普通无分面色导入继续使用原修复流程。颜色快照与导入身份制品的 BREP 不匹配时以 `IMPORT_COLOR_BREP_MISMATCH` 拒绝。

STEP 导出沿不可变 Part/Product 图读取外观，保留多 Body、共享定义与 occurrence 覆盖。每个 Body 的分面颜色针对导出 BREP 重新解析，XDE 根形体标签统一登记面标签，Body 子形体登记整体颜色。RGB 按 8 位 sRGB 交换。原生 BREP 仅含几何，界面明确提示保留颜色需选择 STEP。

## 显示与拓扑变化

`DocumentView.bodyAppearances` 和 `resolvedInstances[].appearance` 是带 `geometryKey` 的临时显示投影，只有分面赋色才需要按需加载 Naming。投影按真实 Modified/Split/Generated/Merged lineage 传播颜色；删除的面不再显示，冲突合并和未解析关系不猜测颜色。未解析计数进入只读面板，STEP 导出以 `APPEARANCE_TOPOLOGY_UNRESOLVED` 拒绝静默丢色。

Three.js 按连续三角形颜色分组复用调色板，保留原三角形顺序及拾取映射；hover/selection 结束后恢复各自底色。分面投影的 GeometryKey 不匹配临时历史显示结果时，只使用 Body 整体色。后端缩略图使用同一外观投影。GLB 仍是可复用的几何显示制品，Body/occurrence 颜色由 Revision 单独投影；直接下载 mesh.glb 不等于带完整文档外观的交换导出。

## 源码与验证入口

- `services/internal/workspace/appearance.go`：模型、命令、导入绑定、lineage 投影。
- `kernel/occt/src/exchange_graph.cpp`、Worker `InspectExchange/ExportExchange`：XDE 颜色交换。
- `web/apps/cad/src/cad/rendering/surface-appearance.ts`、`body-color-dialog.tsx`：渲染与编辑。
- `GeometryExchange.XdeColors*`：整体/分面/黑色、多 Body、共享实例覆盖及重复 STEP 往返。
- `TestBodyColorHistoryAndGeometryIndependence`、`TestImportedAppearanceFollowsLineageWithoutLocatorIdentity`：属性槽与几何独立性。
- `TestXdeImportSharedDefinitionsAndRoundTrip`：真实 Jobs → Router → Worker → Revision → 导出再导入、装配颜色与两步历史。
- Web `surface-appearance` 场景、`browser/body-color.spec.ts`：拾取顺序、交互色恢复、界面编辑/撤销/重做/恢复默认；Mock 浏览器场景不替代真实 API 浏览器验收。
