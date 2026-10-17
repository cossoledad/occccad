# 颜色材质与透明度

外观是 Revision 中的业务数据，与可复用的几何制品分离。当前支持纯色、透明度、粗糙度、金属度、边线样式和可选择性；没有图片纹理、物理材质库或折射。交互入口与个人场景设置见[统一属性](object-scene-properties.md)。

## 模型与继承

- `PartModel.defaultAppearance` 是零件默认；`PartBody.appearance` 保存独立可继承字段，颜色为小写 sRGB `#rrggbb`。`opacity`、`roughness`、`metalness` 为 0–1，黑色、0 和 false 都是显式值。删除字段代表继承。
- `faces[].anchor` 使用 `SemanticTopologyRef`，独立覆盖颜色、透明度、粗糙度、金属度和可选择性。`inheritedFields` 在当前稳定面及其后续 lineage 上清除对应分面字段，回到整体外观；不会清除同源兄弟面的赋色。
- 修改 Body 整体色保留其他字段与分面外观；清除分面覆盖是显式操作。旧 `SET_BODY_COLOR` 也只修改颜色字段。
- Product 的 `appearanceOverrides` 由宿主 Revision 保存 typed `InstancePath`、可选 BodyID、外观和嵌套实例显示别名。不会修改共享零件定义。Body/Face 覆盖核对引用文档，组件替换后不能因复制保留同名 BodyID 而误绑定；失效投影提供 `APPEARANCE_SOURCE_REPLACED`。
- 每字段独立继承：系统 → 零件默认 → Body → 分面；沿装配路径再应用装配默认、导入实例色及宿主覆盖。更具体路径优先，同一路径以外层宿主覆盖为准，Body 范围在整个实例范围之后。整个实例的显式字段覆盖其定义分面中的同字段，其他分面字段保留。透明度是最近显式值，不逐层相乘。

`SET_PROPERTIES` 经 Part/Product 各自版本化 URI，写正式 PropertySlot，并参与幂等 Transaction、CAS、ChangeSet 和 Undo/Redo。外观有独立依赖节点，不进入 B-Rep 几何输入；属性修改、预览和对应历史补偿复用冻结几何，不触发 Sketch/OCCT 或装配约束求解。外观槽以整个目标的外观为补偿粒度；同一目标随后被其他动作修改时，补偿可能报告冲突，不承诺任意字段自动合并。

## 导入与导出

STEP 经 XDE `Quantity_ColorRGBA` 读取实体、分面与组件 RGB/opacity，包括黑色和完全透明。Inspect 输出 definition-local 冻结 BREP；Jobs 将颜色和透明度绑定到同一 BREP 摘要下已分配的拓扑身份，正式模型不保存 OCCT local ID、数组位置或 mesh 面号。

无法证明分面属性与修复后拓扑对应时，以 `EXCHANGE_COLORED_FACE_REPAIR_UNSUPPORTED` 拒绝；外观与导入身份的 BREP 不一致时以 `IMPORT_COLOR_BREP_MISMATCH` 拒绝。无分面属性的导入保留原修复流程。

STEP 导出沿不可变 Part/Product 图解析当前接受的外观，支持多 Body、共享定义与 occurrence 样式。普通导入图保留共享；宿主显式颜色、透明度或分面继承遮罩需要上下文差异时，生成按有效交换外观去重的临时定义变体，保留层级与 placement，不写回业务模型。RGB 按 8 位 sRGB，透明度经 XDE/STEP 交换；不同 CAD 阅读器的显示效果仍需各自验证。粗糙度、金属度、边线、选择性和个人场景设置不属于这条 STEP 交换契约。原生 BREP 仅导出几何。

分面属性沿真实 Modified/Split/Generated/Merged lineage 投影。删除面不再显示，合并冲突、失效引用和未解析关系不猜测。STEP 以 `APPEARANCE_TOPOLOGY_UNRESOLVED` 拒绝静默丢失未解析属性。

## 显示与缩略图

`bodyAppearances`、`resolvedInstances[].appearance` 是带 GeometryKey 的临时投影，包含字段来源、分面值和诊断。解析规则只在单次读取中捕获，不进入运动快照。分面 GeometryKey 不匹配时只用整体外观。

Three.js 按连续三角形有效样式分组，保持三角顺序与拾取映射。纯外观更新复用现有 mesh/BVH，替换材质；选中面使用边界轮廓，避免遮住赋色预览。半透明材质关闭 depthWrite；opacity=0 或禁止选择的面不能由普通拾取命中，可从树恢复。边线可随整体透明度淡化或保持清晰，宽度为 CSS px；线框仍显示拓扑边。浏览器使用 Three.js 常规透明排序，复杂重叠/相交透明曲面可能有排序伪影，未实现 OIT 或物理玻璃。

服务端缩略图使用同一模型外观投影：先不透明深度，再有界像素加权透明合成，最后边线。它不读取浏览器个人场景设置，不保证与实时视口逐像素一致。GLB 仍是几何显示制品，直接下载 mesh.glb 不等于带完整文档外观的交换导出。

## 源码与验证入口

| 范围 | 入口 |
|---|---|
| 模型、字段、lineage、来源 | `appearance.go`、`graphic_properties.go`、`appearance_sources.go`、`occurrence_appearance.go` |
| 正式修改与补偿 | `properties.go`、`properties_history.go`、`properties_preview.go`、`properties_test.go` |
| XDE/RPC | `kernel/occt/src/exchange_graph.cpp`、`GeometryExchange.Xde*`、Worker `InspectExchange/ExportExchange` |
| 完整交换链路 | Jobs `TestXdeImportSharedDefinitionsAndRoundTrip`，含共享结构、嵌套宿主修改、透明度和 STEP 再导入 |
| WebGL | `surface-appearance.ts`、`surface-appearance` scenario、`browser/properties-live.spec.ts` |
| 缩略图 | `services/internal/thumbnail/`、`TestThumbnailTransparencyDepthZeroAndOrder` |
