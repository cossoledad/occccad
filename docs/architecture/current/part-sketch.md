# Part、Sketch 与权威求值

> 返回[当前架构目录](README.md)。代码和测试定义当前事实；验证范围见[验证说明](validation.md)。

整体建模先读[草图与实体流程](part-design-workflow.md)，再按本页进入草图数据/求解、区域、支撑及派生关系。实体能力由独立能力表维护。

Part 中的 `SKETCH` Feature 保存版本化 `SketchFeature v2`：Datum/PLANAR_FACE support、具有稳定 ID 的 Point/Line/Circle/Arc/Ellipse/EllipticalArc/Spline、独立 ExternalGeometry、显式 GeometryRef、Constraint 和最近一次权威 solve 状态。线段、圆弧和开放曲线持有可稳定引用的端点；端点相接必须由 Coincident 明确表达，不能以浮点坐标接近替代模型关系。

## Part 与 Body

Part Revision 保存 Body、活动 Body、几何图形集及有序 Feature。表单打开时冻结目标 Body，预览和确认共用该目标。每个 Body 独立求值、命名与缓存，草图通过真实依赖参与相应链。树可以把输入草图收纳到特征下面，模型始终保留同一个草图身份。所有权、删除及下游失效见[对象生命周期](part-object-lifecycle.md)，制品与冷重建见[求值流程](part-feature-evaluation.md)。

## 草图模型与求解

`EDIT_SKETCH` 原子应用几何与约束，`SolveSketch` 将稳定引用、单位、驱动尺寸和分支意图交给 Geometry Worker。PlaneGCS 在适配层按 DogLeg、Levenberg–Marquardt、BFGS 顺序尝试求解。大幅修改受支持的长度或间距时，可用逐步调整目标取得初值，按最终目标核对残差。预览使用相同的领域验证和求解，产生独立候选；只有正式提交推进 Head。

约束程度由自由度描述，冲突和冗余由独立诊断解释。只读参考尺寸反映已解几何，驱动尺寸通过受检表达式和稳定参数身份进入方程。草图工具将捕获到的端点、曲线、中点等按角色转成正式关系；网格捕获只定位。拖动使用临时目标，取消释放候选。样条控制点与曲线上插值点保留各自语义，构造几何可参与约束并排除在实体轮廓之外。

[求解器与两线距离合同](../../../workers/geometry/sketch/README.md) · [绘制、编辑与约束操作](../../guides/sketch-interaction.md)

## Profile 与 Feature

Profile Builder 排除 Construction/Point，以正式连接等价类构建 Line/Arc/EllipticalArc/开放 Spline 端点图，并把 Circle/Ellipse/闭合 Spline 作为闭环；整草图消费拒绝开放端/T-junction；显式区域可使用与开放/分叉分量独立的正式闭环，含坏端点的整个连接分量不可用，不按坐标补连接或猜分区。循环序列和反向无关的 ProfileLoop identity 保持原合同。生产 Extrude 和 GetDocument 通过 `buildProfileLoops(feature,true)` 只按正式连接构建环，然后向 Worker `ComputeSketchCurves PROFILE` 提交同一组真实曲线，由 OCCT 精确分类外环/孔/岛及验证交叉、重叠和退化；不使用显示采样多边形作为权威区域。OCCT 分类保留输入环原有方向，孔环反向以该原方向为基准，不依赖构造临时面时内核可能调整的方向。ProfileRegion identity 基于外环身份。旧近似多边形 helper 仅用于局部测试/预览。实体求值采用两阶段协议：`LINEAR_EXTRUDE` 或 `REVOLVE` 先从 ProfileRegion 产生临时 Tool Shape，再以 `NEW_BODY / ADD / REMOVE / INTERSECT` 对指定 Body 执行采用、Fuse、Cut 或 Common。OCCT 适配层对空结果、无材料变化、无效 B-Rep 和游离拓扑给出领域诊断；有效多 Solid 结果保留在同一 Body；连续拉伸不再各自产生互相穿透但未合并的实体。旋转轴使用稳定引用，可指向任意 Sketch Line（包括 Profile/Construction 及其他草图中的直线）、AxisSystem 的 X/Y/Z 方向或 DatumAxis；三维参考轴必须位于轮廓草图的支撑平面，服务端将其投影到草图局部框架后再交给 Worker，不能静默使用与轮廓异面的轴。旋转面板作为选择收集器保持打开并等待用户拾取直线或轴；角度、轴引用和反向意图进入 Feature 与求值 digest。当前支持独立多 Body；跨 Body 组合通过独立 Boolean Feature 明确工具输出阶段。轮廓输入语义见下节。

### 显式轮廓区域

拉伸／切除和旋转的 `Feature.Profile` 仍引用上游 Sketch（空间阵列还包含成员槽位），可选 `profileSelection` 区分 `WHOLE_SKETCH` 与 `REGIONS + regionIds`。缺省字段沿用整张草图语义，不迁移旧定义。整草图包含求值时全部合法区域；显式集合只消费保存的区域，后来新增独立区域不会自动加入。集合排序进入定义摘要；依赖仍指向原草图，Preview、提交、每 Body 缓存、Naming 和补偿历史沿现有链路。

区域 ID 复用精确 Profile Builder 的外环稳定身份，普通尺寸修改不改变它。孔归属随权威分类保留，岛是独立候选；不从显示三角形、坐标邻近或候选序号推导持久身份。求值逐项精确匹配，缺失或歧义报 `PROFILE_REGION_MISSING / PROFILE_REGION_AMBIGUOUS`，不会丢弃失败项或退回整草图。上游修改可形成可检查的 FAILED/BLOCKED Revision；显式修复编辑必须处理全部必需区域。原 Feature ID、名称、顺序与 Body 保持不变。

删除并重画外环边会生成新的区域身份：草图重新闭合后，原显式选择仍须重选。视口区分“草图未闭合”和“草图已闭合但区域引用失效”，后者提供“修复轮廓区域”入口；草图编辑中可“结束草图并修复区域”，进入原 Feature 编辑器明确替换失效项。修复内孔边而外环身份未变时沿原区域自动恢复。`profile_reclosure_integration_test.go` 和浏览器 `closed sketch after boundary replacement` 分别覆盖这两种边界及原特征恢复。

新建默认收集显式区域；命令内选择普通草图会把当前全部区域冻结为集合，面板列出选定区域。视图区按后端 `SketchProfileAnalysis.regionViews` 提供的外环／孔显示采样面拾取、高亮和增减，采样只用于显示，提交重新经过精确 Builder。编辑恢复有效项并保留失效行，支持明确替换或移除；“全选当前区域”是用户主动替换集合。整草图与显式集合由用户切换，不自动转义。候选限定目标特征之前的同文档、Revision 和 occurrence；预览及取消不推进 Head，变更集合不能采用旧候选。

源码入口：`profile_selection.go`、`sketch_exact_profile.go`、`solid-feature-editor.tsx`、`profile-region-selection.ts`。定向真实 Router/Worker 回归 `profile_region_selection_integration_test.go` 覆盖独立／多区域、孔及岛、增加第三区域时 BREP/Naming 保持、尺寸更新、失效替换／移除、下游恢复、取消、Undo/Redo、重开和冷重建；浏览器 `solid-feature-live.spec.ts` 的 `explicit profile regions` 覆盖实际区域拾取、孔内过滤、编辑、修复及拉伸／切除／旋转体积。显式区域先从正式连接图提取独立闭合分量，再由同一精确 Worker 分类外环/孔/岛；开放线和分叉的诊断仍保留。`sketchAnalyses` 在 OPEN/INVALID 时也可提供已精确验证的 `regionViews/regionIds`，`geometryVerified` 仅认证这些候选，不表示整张草图闭合。WHOLE_SKETCH 和放样仍采用严格完整轮廓检查；闭环集合的交叉/重叠/自交仍由精确门禁拒绝。未支持任意相交曲线自动分区、开放实体轮廓。空间草图阵列成员继续使用已有整草图输入，尚未提供成员的区域拾取投影；放样未扩展。

## 支撑与外部几何

独立 Part 的历史补建草图及后续有限拉伸见[历史位置创建](solid-features.md#在历史位置创建)。拓扑投影保存真实来源阶段及 Revision；当前视图的拾取必须来自草图之前的精确前缀，显式冻结的较早版本须有原始证据且其真实阶段仍在草图之前。二者均不以最终 Body、displayFallback 或坐标近似代替来源。

基准面/轴沿用已有 Datum 身份和框架，支持自由坐标及关联定义。关联定义保存来源基准、绕引用直线旋转、沿引用方向平移；先按右手方向旋转原点与框架，再平移。标准面/轴、自定义基准、上游草图直线和精确实体平面/直线边按角色解析；实体来源保存 PersistentSelection、source Revision 和明确的 Body 特征阶段。角度和距离分别使用 `datum:angle / datum:distance` 的稳定 ParameterDefinition/checked AST，数值框架是求值结果。现有依赖图拒绝缺失、循环及通过基准间接读取下游阶段；失效引用不回退为自由框架。创建/编辑复用权威 PreviewCommand，取消不写 Head；参数更新将最终基准框架和草图支撑纳入已有 ChangeSet、CAS、Undo/Redo。树中自定义基准可编辑/删除，标准基准与被引用基准受保护；普通单击仍只选择。

参考点/线/面的构造编辑统一支持数值、带单位字面量和表达式，来源使用已有 ParameterDefinition/checked AST；坐标与方向采用一行向量布局，参数引用在同一输入中完成。选择实体平面、直线边、圆/椭圆或球中心前按角色检查精确类型，绑定保留 PersistentSelection、源 Revision 和 Body 阶段。新建对象没有 ID 时跳过自引用比较，已有对象仍禁止引用自身。输入问题留在对应命令中，不与全局通知重复呈现。

Origin 的轴系增加显式原点入口，标准 X/Y/Z 与原点的树/视图选择共用文档、Revision 和 occurrence 身份。Part 原点随实例位姿变换，不是装配世界原点。

Sketch support 使用明确的右手框架。默认／自定义基准面保存 `origin + normal + uDirection`；实体面支撑保存 PersistentSelection、来源 Revision、Body、明确的 `sourceFeatureId` 阶段、origin/X/normal、定向规则和依赖快照。来源阶段取自实际上游 Body tip，不用语义 Anchor.FeatureID 替代；解析读取目标草图之前的完整 Body 前缀，并以保存的来源阶段重建历史 Naming。采用实际实体面的有向法向，将保存的 X 方向投影到新平面，退化时沿用确定的稳定方向规则，不翻转法向或退回默认平面。Visualization、拾取、草图编辑与 Profile 共用该框架；依赖保留基准、拓扑来源、来源阶段及当前前缀的真实关系。

结构树的 Sketch 对象菜单“更换草图支撑…”对正常和失效草图均开放。正式 `REASSIGN_SKETCH_SUPPORT` 经权威阶段绑定，转换为现有 `EDIT_SKETCH` 的 `REASSIGN_SUPPORT` 操作，沿用 `sketch.model` ChangeSet、Preview/Commit、CAS 和补偿历史。唯一定位语义为“保留局部二维设计，随新支撑移动”：Sketch、Entity、Constraint、Parameter ID、Body、名称和历史位置保持；局部拓扑不变时原显式区域选择继续有效。标准／自定义基准面及同 Part 的合法上游实体平面均可重选，包含跨 Body；新选择不要求旧支撑解析成功。预览显示重投影后的草图和新框架的法向／X 方向，输入或 Revision 变化使旧候选失效，取消不写 Head。

已有面支撑因上游合法修改而缺失、歧义或变为非平面时，保留原定义并提交可检查的 `FAILED_SUPPORT`，依赖后代为 BLOCKED，独立 Body 继续求值。上游实体本身失败时标记实际失败 Feature，相关草图为 BLOCKED，保留实际阻塞位置；权限、制品读取和服务错误仍原子拒绝。非法新支撑、下游／循环依赖和显式重指定后的投影或求解冲突也原子拒绝。失效支撑不消费旧投影快照；外部关联保留身份，按新框架重新投影和求解，不自动 Detach。关联基准自身求值失败仍沿既有基准合同原子拒绝，不把其旧数值框架当作新支撑。原有被引用对象删除保护保持。

源码入口为 `sketch_support_edit.go`、`sketch_support.go`、`document_evaluation.go` 和 `sketch-support-editor.tsx`。`sketch_support_reassignment_integration_test.go` 通过真实 Router/Worker 验证支撑更换、源面真实消失、来源阶段与 Anchor 不同、跨 Body、外部重投影／约束冲突、非平面与循环拒绝、失败传播、删除保护、原身份、区域保留、取消、Undo/Redo、重开、冷重建，以及客户端断连不写 Head。浏览器 `solid-feature-live.spec.ts` 的 `sketch support changes` 实际使用对象菜单、基准树选择和失败 Body 的上游平面拾取，验证预览取消、修复、历史与重开。当前证据为共享 SQLite、Release Worker、Chromium/SwiftShader；复杂曲面变化、装配 occurrence 及目标设备实机未覆盖。未扩展保持世界位置模式、跨文档支撑或通用基准失效修复。

ExternalGeometry 与普通 Sketch Entity 分开持久化。每项保存稳定 ExternalId、Edge/Vertex PersistentSelection、source Revision、`ORTHOGONAL` projection kind、resolved source digest、dependency snapshot 和 Worker 生成的二维 Point/Line/Circle/Arc/Ellipse/EllipticalArc snapshot；Constraint 通过 `EXTERNAL + ExternalId + stable sub-element` 引用它。控制面固定按 naming resolver → Worker projection → Sketch solve → downstream Feature 顺序处理，求解边界把外部几何作为内部 fixed geometry 输入 PlaneGCS，但不会把求解结果写回它。`DETACH_EXTERNAL_GEOMETRY` 将当前已连接 snapshot 原子冻结为同 ID 的普通 Construction Entity 并改写引用；Reconnect 只替换 PersistentSelection，保留 ExternalId。显式 ADD/RECONNECT 若无法完成权威投影，会以 `EXTERNAL_GEOMETRY_PROJECTION` 阶段和 Worker 稳定诊断原子拒绝，旧 Head 不变；只有先前已连接的来源因上游更新而缺失、歧义、类型变化或投影退化时，才清除旧 snapshot、列出受影响 constraint/profile/downstream Feature 并提交可检查、可 Reconnect 的 FAILED Revision。FAILED Revision 不得把 dependency snapshot 的上游 prefix geometry key 冒充自己的最终制品；读取时只生成与当前模型 Manifest 匹配的诊断可视化，因此失败状态仍可继续检查和编辑。圆与椭圆 Edge 使用 Naming 中的精确半径、轴、X 方向和参数区间做正交投影：修剪区间保留为 Arc/EllipticalArc，斜投影圆得到精确椭圆，反向框架保留同一修剪轨迹；侧向退化的圆锥曲线用包含内部极值的线段表示。圆心是同 ExternalId 的 CENTER 子元素，支持显示、捕获和约束，不另建持久 Point。面边界组、Section、Silhouette 和一般 BSpline 投影未交付；当前精确修剪圆/椭圆弧投影不能被列为待实现。

草图投影工具通过统一选择输入接收结构树与视图区的原点、标准轴、基准轴、实体边和顶点；仅接受当前编辑文档、Revision 与 occurrence。启动/退出工具时刷新基准显示，投影期间临时显示当前 occurrence 的基准，退出恢复普通草图显示。普通草图拾取策略不扩大。

投影支持视图区 Delete 和结构树删除，复用 `DELETE_ENTITIES` / `DELETE_NODE(S)`；只删除当前 Sketch 的 ExternalGeometry 定义及其草图内依赖关系/尺寸，不删除来源几何。失败投影同样可从树删除，无须先恢复旧快照。尺寸参数生命周期、跨参数依赖校验、原子提交及 Undo/Redo 沿现有链路，不能通过删除投影绕过依赖保护。入口见 `sketch_geometry_edit.go` 与 `sketch_followup_integration_test.go`。

原点、标准轴和基准轴投影在同一 ExternalGeometry 中保存 DatumReference，复用 ProjectExternalGeometry RPC。方向斜投生成无限支撑线，垂直于草图面时生成点；有限直线实体边保留真实参数域；垂直投影生成关联 Point，PersistentSelection 仍是原 EDGE，零长度/非法方向源仍拒绝。方向快照的两个点仅编码规范单位方向，不是边长或可拾取端点；视图区按屏幕长度显示，约束/捕捉不提供伪端点、中点、长度或相等关系，也不能将轴断开成任意有限线段。角度/高度更新后重新解析方向与二维快照。创建与重连要求当前所有者 Revision；跨 occurrence 输入由同一选择上下文门禁拒绝。

草图编辑的 ChangeSet 以最终写入 Revision 的求解后 `sketch.model` 为准，而不是命令处理器产生的求解前候选值；历史投影层能够独立读取和回写该稳定属性槽。Undo/Redo 对持久 ChangeSet 先验证稳定 write-set 的 target/slot 唯一性，再从原事务不可变的 base/result Revision 重建实际 before/after 和 digest，最后执行当前值冲突检查；因此旧版本中已写入错误 digest 的求解后草图事务也能修复并回滚，但不会信任旧 ChangeSet 内容或放宽并发冲突检查。PlaneGCS 改写坐标、DoF 或诊断后，补偿和重放不会再产生候选值与 Revision 的 digest 冲突。

GeometryId 是精确 Body B-Rep 的 SHA-256 内容标识，不绑定 Worker；`geometry_key` 标识带 evaluator 策略和可视化几何内容的求值结果；纯显隐元数据及可读名称不进入该键，因此两个结果可以共享 GeometryId，但拥有不同的可视化制品。几何输出包括 B-Rep、GLB、三角形、边折线、包围盒、拓扑计数和体积。几何大数据仅保存在 ArtifactStore/S3；数据库保存轻量索引与摘要。Body 的 ADD/REMOVE/INTERSECT 在 OCCT 布尔完成后统一同域面和同域边，再进行 B-Rep 校验和内容寻址；因此相交且等高的拉伸不会把连续顶面暴露成多个共面选择区域。

命名、history、旋转与导入根命名及完整 Shape gate 见[持久命名](persistent-naming.md)和[实体 Feature](solid-features.md)；只有实际通过完整命名门禁的制品可以成为持久拓扑来源。

几何驻留按不可变 GeometryKey/GeometryId 路由，详见[Router 与制品](jobs-artifacts.md)。

活动 Sketch 的原点和 U/V 轴是稳定的内置 GeometryRef，而不是临时渲染对象：原点参与点类签名，U/V 轴参与直线/求解曲线签名，因此 Coincident、Parallel、Perpendicular、Tangent、PointOnObject、Angle、Symmetry 和点线 Distance 共用同一选择与服务端验证语义。线性尺寸覆盖线长、点点真实距离、点到无限支撑直线/U/V 轴距离、平行线间非负支撑线间距，以及有符号 ΔX/ΔY（第二引用减第一引用）。平行线间距使用可见 PARALLEL 关系，已有平行关系不重复创建；它不是有限线段最近距离。椭圆轴尺寸为主/次半轴的毫米值。圆与圆弧中心、圆弧和开放 Spline 端点均作为独立点标记显示和拾取；Line、Arc、Polyline、Spline、Rectangle 与独立 Point 命中已有稳定点时，会在同一原子编辑中写入显式 Coincident。结构树双击 Sketch 直接进入该 Sketch 的编辑上下文。

Sketch Entity 的 `PROFILE`/`CONSTRUCTION` role 是持久领域状态。结构树右键可在“轮廓元素/构造元素”之间切换，操作形成普通 `EDIT_SKETCH` Transaction，经过权威求解、最终 ChangeSet 和 Undo/Redo；Profile Builder 只消费 `PROFILE`，因此构造线、构造曲线和构造点不会进入 Pad。活动 Sketch 的 U/V 轴与原点采用相同的参考几何语义，但不作为可写 Sketch Entity 持久化。权威 VisualizationManifest 为 Circle/Arc 生成中心点、为 Arc 生成端点、为 Spline 生成全部拟合点；Select 工具拖动 Arc/Circle 中心或 Spline 拟合点时只显示瞬态点预览，并在 pointerup 提交一次 `UPDATE_ENTITY_POINT`。

Part 交互在退出 Sketcher 后把选择提升为整个 Sketch Feature，并保持未被实体特征消费的草图可见；已消费 profile 仅在重新编辑时临时显示。视图区在 Sketcher 外命中草图点、线或约束时同样投影到整个草图，因此可以直接继续 Pad/Pocket/Revolve。新建实体特征成功后自动选择结果 Feature，保持“选平面→建草图→绘制→退出→拉伸”的连续操作链。

草图“移动”命令共用平移与旋转手柄，不再提供独立旋转命令。`DRAG_ENTITIES` 保存冻结基线上的绝对目标意图；控制面向现有 `SolveSketch` 传入临时坐标目标，PlaneGCS 用负 tag 的辅助目标在正式约束可行域内求解。目标不持久化为约束、不改变公式/尺寸定义、不扣减正式 DoF；固定、方向及跨选集连接允许限制目标或带动邻接几何。内部计算结果带源几何/约束摘要，提交验证稳定身份后经普通 ChangeSet、CAS 和 Undo/Redo。复制和显式刚性 `TRANSFORM_ENTITIES` 继续使用原有内部/跨选集约束合同。

移动预览单飞合并最新绝对目标，显示已求解的可见几何；松开手柄等待最终目标的只读候选，再原子提交一次。求解失败保留选集，未知提交结果沿原请求回执恢复。手柄中心只捕获可见草图元素，不计算背景网格吸附，也不覆盖绘图工具的吸附引用。Worker Proto 新增临时 `SketchModel.drag_targets`，服务端与 Worker 必须使用匹配构建；持久 Sketch 格式不变。

多边形创建以 `CREATE_POLYGON` 原子操作接收中心、参考圆半径、方向、3–50 整数边数及内接/外接模式。模式采用 Onshape 参考圆的命名：INSCRIBED 的边在圆外相切，CIRCUMSCRIBED 的顶点在圆上。控制面生成确定身份的普通线段与 Construction 圆，显式闭环 Coincident、Equal、Tangent/PointOnObject 保持其圆关系；中心吸附成为显式 Coincident。参数修改、Profile、拉伸及历史沿现有求解链，未新增特殊多边形实体类型。

DocumentView 的 `sketchAnalyses` 是统一 Profile Builder 的只读分析投影。开放端、分支、重复、退化和失效连接携带实体/引用/模型位置；Web 只展示后端分析，不另建闭合判定；仅 `geometryVerified=true` 的闭合结果显示检查通过，未精确验证的连通闭环明确显示等待验证。合法开放草图可以保存；封闭实体拉伸在消费 Profile 时拒绝开放边界。闭合状态与求解约束程度独立。交互与验证入口见[草图交互](../../guides/sketch-interaction.md)。

## 实现与验证入口

[Part 关联测试](../../../services/internal/workspace/associative_design_test.go) · [Profile Builder 测试](../../../services/internal/workspace/profile_builder_test.go) · [Worker 协议](../../../proto/occccad/worker/v1/geometry_worker.proto)

面上建草图按最终 Solid 中的 Face 朝向读取精确法向，保持右手框架。编辑预览保留有效解和稳定引用；切换文档或 occurrence、取消及迟到响应均不能推广旧候选。

## 草图关联阵列

`SketchFeature.patterns` 保存选定种子 EntityId 和与[实体/空间阵列](feature-replication.md#参数化阵列)共用的分布定义。PlaneGCS 只求解种子；Profile 与显示消费解后派生成员，不复制成员驱动尺寸或 Fixed 约束。一个有效阵列最多派生 4096 个实体，同一源实体不能同时归属多个启用的草图内阵列。二维阵列必须保持草图平面。

`EDIT_SKETCH` 的 `CREATE_PATTERN / EDIT_PATTERN / DELETE_PATTERN / DETACH_PATTERN` 是原子领域操作，阵列参数拥有稳定 managed ParameterID。成员默认只读；`DETACH_PATTERN` 才生成独立几何并复制必要的 Coincident 连接，原种子转为 Construction。删除阵列恢复普通种子，不保留无关联成员。选中种子或任一派生成员执行 Delete、删除工具或框选删除时，`DELETE_ENTITIES` 在权威端归并为删除所属阵列定义；同阵列多成员只删除一次，混选普通几何仍正常删除。派生成员的 `sketchPatternMembers` 是 DocumentView 显示投影，不进入持久模型或求解自由度。

草图内阵列的中心保存当前草图原点、点、端点或圆心引用；线性方向保存当前草图 U/V 轴、直线或已连接的投影直线引用，并支持反向。引用只能来自当前草图及其投影快照，不直接引用世界轴或其他草图；视图区使用草图引用的 hover/选中反馈，中心辅助点不生成新几何。参数投影与种子求解后重新解析引用；不存在、已抑制或不在草图平面内的引用明确失败。阵列定义及只读派生几何进入结构树，定义支持编辑、抑制和删除，成员可拾取和高亮。成员约束颜色使用源几何所在求解分量，不受无关自由几何影响；独立修改仍需解除关联。

草图阵列新建时线性默认绑定 U 轴、圆周默认绑定草图原点。阵列通过既有草图引用选择器处理 hover 与按下确认，普通点/尺寸拖动和框选不接管指针；种子几何复用草图屏幕空间拾取。当前 hover 候选使用蓝色，已绑定的默认轴、原点或种子不以选中色遮盖候选；移出后恢复选中反馈。树删除通过 `DELETE_NODE / DELETE_NODES` 入口校验所属草图后进入同一原子删除逻辑。

## 当前限制与修复边界

- 拉伸／切除、旋转和 Sweep 支持[显式区域集合](#显式轮廓区域)；空间草图阵列成员尚无区域拾取投影。
- 草图支撑主动更换与失效重指定见[支撑与外部几何](#支撑与外部几何)；关联基准自身失效仍沿原子拒绝合同，未引入通用基准修复工具。
- 当前草图的精确修剪圆/椭圆弧投影已有实现；面边界组、Section、Silhouette、一般 B-spline 投影未交付。完整样条曲率/G2/G3、通用三维线框/曲面建模也不属于当前支持范围。
- 当前实体范围与修复由[唯一能力表](solid-features.md#feature-能力表)、[生成器算法](solid-generators.md)和[特征复制](feature-replication.md)维护；已包括 Sweep、更多关联终止、薄壁、工程孔、受限放样连续性及生成器成员重执行。跨 Body/任意非连续集合移动、任意历史插入、任意局部特征重执行仍不支持。

交互验证入口：`sketch-capture-angle.scenario.mjs`、`services/internal/control/sketch_interaction_integration_test.go` 与 `solid-feature-live.spec.ts` 的 `sketch interaction` 场景。分别验证捕获/选择状态与迟到响应、真实 Router/Worker 的精确分割/角度求解与历史、浏览器拾取/尺寸/取消及重开；测试存在不代表当次已执行，结果以交付记录为准。

## 关联偏移

`Sketch.Offsets[]` 保存稳定定义 ID、普通本地源实体 ID、有符号距离、MITER/ROUND 连接及抑制状态。距离拥有稳定 `parameter:<sketch>:offset:<id>:distance`，复用 Quantity、受检表达式、依赖图和原子 EDIT_SKETCH；默认偏移工具创建关联定义，也可显式选择独立副本。结构树双击定义可更换源曲线、编辑距离/连接、删除或解除关联；源曲线也可在视图区点选增减。

求解器只求解普通源几何，偏移结果在同一接受快照上通过原精确 Line/Circle/Arc 算法派生，生成 ID 由定义和源身份决定；不是隐藏固定约束，也不承诺双向驱动。统一 `sketchDerivedEntities` 投影供显示、只读检查与树使用，轮廓 Builder 使用相同派生几何及正式端点连接。生成曲线不可直接拖动、标注或修剪；须显式解除关联，保留输出实体 ID 与连接后转为普通几何。关联距离被其他表达式引用时，删除/解除关联沿参数缺失检查原子拒绝，须先处理依赖。

源尺寸变化导致退化、自交或断链时，保留定义并报告 OFFSET_FAILED，不复用旧输出；整草图生成失败，显式区域仍可使用其他有效组件。删除仍被引用的源曲线须先删除或解除偏移。Ellipse/Spline 精确偏移、派生结果作为另一偏移源及双向 GCS 约束尚不支持；每草图最多 4096 个偏移输出预算。回归入口 `TestLinkedOffset*`、`sketch-edit.scenario.mjs` 与真实浏览器关联偏移场景。

## 方向推断与草图诊断

普通直线在绘制时按物理夹角和 CSS 像素距离共同推断 H/V，以及邻近普通本地直线的 Parallel/Perpendicular。状态区先显示候选关系和支撑，确认后创建普通正式约束；候选沿现有合并预览队列求解，确认复用同一几何和约束 ID。已有端点捕获优先，避免叠加另一方向意图。工具选项可单独关闭方向推断或关闭自动连接/推断；Alt 临时关闭，确认时再次检查当前按键状态。推断只扫描附近最多 128 条普通直线；不承诺远处几何或任意派生输出的自动方向关系。

“草图诊断”读取接受快照的整体/分量 DoF、约束程度、冲突、冗余及抑制状态，沿用装配诊断的问题/约束/自由度表格。稳定 Entity/Constraint ID 用于选中高亮、定位和编辑；停用/启用走 EDIT_SKETCH 与历史链，生成关系只读。UNRESOLVED 零计数不显示为完全约束，冗余与约束程度保持正交；当前未返回零空间方向向量，不能把分量定位解释成独立拖动方向。

绘图与编辑候选的作用域包含 Part Revision、Root Product Revision、ContextVariant、occurrence 和活动草图；仅 Part Head 不变不足以继续使用旧上下文候选。入口为 `sketch-direction-inference.ts`、`sketch-diagnostics.ts`、`SketchDiagnosticsPanel`；真实求解回归 `TestSketchDirectionInferenceSolvesThroughRouter`，交互回归 `direction inference is visible`。
