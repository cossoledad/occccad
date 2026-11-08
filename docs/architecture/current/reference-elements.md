# 参考元素与几何图形集

Part 提供共同“参考元素”工具栏：参考点、参考线、参考面、几何图形集。标准 XY/XZ/YZ Plane 直属 Part，绝对轴系保留原点和 XYZ 子轴。Product 内通过既有编辑会话创建/修改 Part 定义，不创建脱离文档归属的全局几何。

源码从 `workspace/reference_elements.go`、`reference_construction.go`、`reference_point.go`、`datum_definition.go` 和 Web `reference-element-editor.tsx` 进入。CATIA 对照来源见[参考手册](../../references/catia-measurement.md#参数与参考元素)。

## 构造与参数

| 类型 | 当前构造方式 |
|---|---|
| 点 | 三维坐标、两点之间的比例、圆/球等解析中心、平面内 U/V |
| 线 | 两点、点与方向、过点垂直于平面、原点与方向分量 |
| 面 | 平行偏移、过点平行、三点、绕轴成角、平面方程、原点与法向分量 |

所有构造保存显式模式及 typed `DatumReference`。拓扑输入先绑定 `PersistentSelection`，保存来源 Revision/Feature 阶段，权威解析器继续验证类型、阶段、缺失/歧义、有限性、零方向及共线等退化。不会以显示坐标、名称或 mesh 序号修复关联。现有 DatumTransform 旋转/平移模型保留自己的编辑器；与新构造模式互斥，共用参数、图检查及精确来源解析。

数值槽由所属 reference ID 和 `reference:*` 槽生成 `DATUM_REQUIRED` 参数，SI 数量或无量纲方向分量。表单支持字面值/表达式与参数插入，预览和正式提交走相同命令适配。编辑字面值必须写参数来源，不能仅改计算后的坐标。参数改动传播到参考元素及消费者，图循环仍在提交前拒绝。树下展示该元素自己的参数；参数及构造创建、删除、修改参与 ChangeSet、补偿与重放。已被其他构造、公式或发布使用的对象不能直接删除。

线是无限构造方向，显示长度只用于视区，不承诺两个端点之间的有限曲线实体。平面为无限解析支撑，显示板尺寸也不形成几何面积。点用独立 Points primitive，预览、正式显示及稳定选择共用相同归属。独立 Part 统一调用参考几何显示入口；Product 通过 `ReferenceDisplayScope` 携带接受的 Part Revision、occurrence、位姿和参考定义，在没有实体制品时仍可显示参考元素；全局选项可分别控制点、线、面、轴系，持久隐藏祖先不能被拾取字段揭示。

## 集合与历史

`PartModel.geometricalSets[]` 是真实容器，具有稳定 ID、名称、可选 ParentID 和可见性。参考点/线/面、[几何 Feature 及其上下文新建的 Sketch](wireframe-surfaces.md) 保存 `geometricalSetId`；首次创建可在同一命令中建立默认集合。表单可以选择已有集合。集合可新建、改名、改父级和删除空集合，校验父级存在、兄弟名称和循环。非空检查同时覆盖参考元素、草图和几何 Feature；成员变更不以复制几何或修改其稳定身份实现。当前没有拖拽成员搬移或解散非空集合命令。

集合是无序组织，不是实体 Body 的材料求值序列；全 Part 参数/参考/Feature 依赖决定计算顺序。标准平面不进入集合；几何构造由 Feature evaluator 执行。集合的纯组织或显隐变更复用既有几何制品。

## 边界与验收

当前不是 CATIA 全部 Point/Line/Plane 构造集合：参考构造本身不包含曲面内点、任意切线/法线、独立坐标系选择或任意跨文档直接输入。有限线段和曲线上点由几何 Feature 另行提供，不能混同无限参考线。测量仍只接受其自身支持集合，创建参考元素不自动扩展测量的 Datum/Sketch 支持。

命令链验证入口为 `TestReferenceElementsCommandPreviewParametersReopenAndHistory`，数学与树合同由邻近 reference 测试覆盖。浏览器验收在 `measurement-live.spec.ts` 的 reference 场景中覆盖真实预览、树拾取、编辑及重开；测试存在不代表当次已执行通过。
