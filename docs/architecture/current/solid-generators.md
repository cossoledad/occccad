# 实体生成器

返回[唯一能力表](solid-features.md#feature-能力表)。本页维护生成算法、参数与引用、资源边界及对应检验入口；共同的预览、历史和修复合同见[实体 Feature](solid-features.md)。

## 自由放样与截面对应

Loft 使用真实 OCCT AddVertex 点端和精确轮廓；自动比较循环起点、方向和圆相位后保存 stable seam、反向、相位与 correspondenceResolved，重算不重新猜对应。法向符号按截面位移统一，框架比较不移动输入。上限为64条轮廓边、一亿次候选比较；自交或构造失败拒绝。

## 独立 Hole

`HOLE` 是一个独立 Feature，支持普通孔、圆柱沉孔与锥沉孔，复用创建/编辑、Preview/Commit、ChangeSet 和历史链。定位保存同 Part 合法上游普通 Sketch 的单个或明确多选的显式 POINT 稳定引用，轴沿已求解支撑的有向法向，可反向；不保存视口坐标，不生成隐藏圆草图或额外切除 Feature。孔位通过原草图尺寸/约束修改，多点草图必须明确选点；端点、圆心、原点与阵列派生点不属于定位输入。

三种孔都管理主孔直径，平底盲孔管理从定位平面起算的深度；钻尖深度另有明确约定；单向贯穿只切除该平面沿选定方向的目标 Body 材料，不改变普通拉伸的贯穿语义。圆柱沉孔增加沉孔直径和沉孔深度；锥沉孔增加沉孔直径和**完整锥面夹角**（0° < α < 180°），锥段深度为 `(沉孔直径 − 主孔直径) / (2 tan(α/2))`，面板显示派生值。沉孔直径必须大于主孔直径，沉孔深度必须为正，盲孔沉孔段必须短于完整圆柱深度；非法组合、无材料交集和无效结果拒绝。

所有活动尺寸沿用 Quantity、单位、表达式、依赖和 managed ParameterID；孔型/终止模式切换移除非活动参数，共有参数身份及自定义别名保留，Undo/Redo 恢复原定义。evaluator 构造一个精确切削工具，由主圆柱与原生圆柱/圆锥通过真实 OCCT 历史融合，再进入既有 Boolean/Naming。主孔壁、沉孔壁、台阶、锥面、孔口和孔底按实际构造与裁剪命名，保留定位草图与点的完整来源身份；确定的尺寸修改延续引用，孔型切换后消失的角色保留失败定义，要求明确重选，不自动换面。Hole 编辑若仅下游失败，预览显示权威孔阶段并指出实际待修复特征，提交仍完整求值；孔自身或上游失败不允许确认。

定位草图、点实体和目标上游阶段进入依赖、删除/循环检查及缓存；合法删点留下 `HOLE_POINT_MISSING`，下游按依赖阻塞。编辑器保留待修复输入并提供“修复孔位”；草图支撑失效沿[已有支撑重指定](part-sketch.md#支撑与外部几何)修复。取消和预览不写 Head，修复保持 Hole ID、名称、Body 与历史位置。同 Body 的合法 `BLIND` / `THROUGH_ALL` 三种孔型可作为 `GENERATOR_TOOL`，执行线性／圆周阵列与镜像，详见[参数化阵列](feature-replication.md#参数化阵列)。单向贯穿按成员重建主孔切削范围，入口沉孔保留尺寸；有限 BLIND 可与后续修饰组成净切除 `FEATURE_DELTA`；显式 `BODY_STAGE` 保持复制整个已求值阶段的原意。到平面 Hole 不作为固定复制工具，可选成员重执行；钻尖、孔组与工程螺纹标注见下文。

源码入口：`workspace/hole.go`、`evaluation_projection.go`、`occt_kernel.cpp` 的 `add_hole_recess` 与 `solid-feature-editor.tsx`。定向回归入口：`GeometryExchange.Hole*`、`TestEngineeringHole*`、`TestHoleLifecycleThroughRouter`、`TestHoleToPlane*` 和 `solid-feature-live.spec.ts` 的 Hole 场景；覆盖精确体积/方向、表达式、引用延续/失效修复、抽壳外壁支撑、历史与冷重建。执行结果以当轮交付为准；当前环境是 SQLite、Release OCCT 7.9.1 与 Chromium/SwiftShader，不代表 PostgreSQL 或全部几何组合验收。

## 到平面关联终止

单方向直线拉伸／切除及 Hole 的 `TO_PLANE` 目标为标准/自定义基准面，或同 Part 合法上游实体平面的无限延伸支撑平面，包含已有跨 Body 场景。显式区域集合、外环和孔沿用精确 Profile Builder；目标不受有限面的边界限制。定义保存 `terminationPlaneId` 或 `terminationPlane`（PersistentSelection、来源 Revision、Body 与实际来源阶段），不以语义 Anchor 代替阶段，也不保存一次测得的长度作为终止条件。

Hole 主孔从定位草图平面沿所选法向终止，入口圆柱沉孔/锥沉孔保留尺寸；不保存测得孔深，也不创建深度参数。完整精确入口工具必须严格位于终止面之前，平面截入沉孔段或较宽孔口也拒绝。外部目标先经 Publication → ContextInput → 本地基准面接入，不直接绑定外部实体面。

每次求值从当前合法上游前缀解析目标框架；Proto/Worker 接收当前精确平面，OCCT 以棱柱和支撑半空间求交形成平行或倾斜端面。每个选中区域的所有点沿指定方向都须有正终止距离；平行、反向、穿过起始轮廓、零厚度或无效结果明确失败。端面、侧面与边/点使用真实生成及裁剪历史进入原 Naming 门禁。目标移动或旋转进入依赖和缓存，删除、循环及非平面目标由权威校验拒绝。

面板明确显示“到平面”，可拾取和重选基准面/实体平面。失效目标保留原定义及解析状态，失败提示提供“修复终止平面”入口；候选来自当前特征之前的合法阶段，不能绑定下游或 displayFallback。显式修复须解析全部必需输入，沿原 Preview/EDIT_FEATURE、ChangeSet 与历史提交，保持 Feature ID、名称、Body 和位置。合法上游变化可留下可检查的 FAILED/BLOCKED；服务、权限、制品错误继续返回实际错误。圆边变为椭圆等几何类型变化仍按现有引用合同要求明确重选。

`GENERATOR_TOOL` 和 `FEATURE_DELTA` 不接受此终止模式，线性/圆周阵列及镜像共用此能力限制。`BODY_STAGE` 仍可刚体复制已经求值的实体阶段，不在每个成员位置重新终止。双向关联终止、有限面的修剪边界终止仍未支持；直线生成器的其他关联模式见下节。

定向入口：`services/internal/control/to_plane_integration_test.go` 的真实 Router/Worker 生命周期、目标失效与修复、历史/冷重建；`GeometryExchange.ToPlane*` 的精确体积、内孔、多区域、正反方向、Naming 与非法距离；`solid-feature-live.spec.ts` 的 `to-plane` 实际拾取和修复。

## 到下一面、曲面与偏移

直线拉伸/切除增加 `TO_NEXT`、`TO_SURFACE`。前者消耗当前目标 Body 的实际上游实体，逐选中区域终止到整个起始轮廓前方的相邻边界；起始轮廓位于材料内部时终止于下一出口。它不保存一次拾取的面，要求已有目标 Body，不支持 NEW_BODY。后者保存独立 `terminationSurface`（同 Part、上游 PersistentSelection、来源 Revision/实际阶段），每次先解析当前前缀，再把完整 BREP/Naming 交给 Worker；可以引用其他 Body 的精确底层曲面，修剪边界不作为终止限制。平面复用无限平面算法；非平面用底层曲面分割。当前验收包括圆柱终止及上游半径变化，不代表所有无界曲面/延展或复杂裁剪组合均可成功。

OCCT 精确分割后，只接受与整个起始端面相邻的唯一实体单元；若远端棱柱盖仍存活，则轮廓未被完整终止。穿过起始轮廓、无唯一单元、没有有效端面历史、部分覆盖、反向、零厚度、无效实体及命名不完整均失败，不靠有限采样或最近面猜测补齐。下一面输入最多 4096 个命名面；达到预算明确拒绝。多个区域独立求终止后进入原材料操作与完整 Naming 门禁。

`startOffset`、`endOffset` 是正式长度 Quantity/表达式，保存稳定 `parameter:<FeatureId>:<slot>`。正值沿已反向处理的拉伸方向，负值沿相反方向；起始偏移移动草图构造平面，有限/双侧/对称长度仍定义实际扫掠区间。终止偏移沿拉伸方向平移已经确定的终止边界，正值延长、负值提前，不是曲面法向偏置；再次精确分割验证。有限模式不暴露终止偏移；关联模式不暴露有限长度；既有贯穿切除保持其范围合同，不提供起始偏移。Hole 不接受这两个直线生成器字段。

面板选择模式、目标与带单位偏移，预览不推进 Head；编辑保留 Feature/Parameter、Body 和历史位置。目标失效保留原引用，可在输入阶段显式重选；同位置、同形状的另一个 Body 不会自动代替。失败提示提供终止限制修复入口。关联生成器暂不充当 GENERATOR_TOOL/FEATURE_DELTA 种子，BODY_STAGE 仍可复制结果；通用成员重执行仍是后续任务。

验收入口：`ExtrudeOffsetsAndNextTerminationUseEntireExactProfile`、`ToSurfaceUsesImmutableSelectedUnderlyingCylinderAndNaming`、`ToSurfacePlaneExtendsBeyondFiniteFaceBoundary`；真实 Router/Worker 的 `TestAssociatedExtrudeLifecycleThroughRouter` 与 `TestCurvedTerminationUpdatesAndRequiresExplicitRepairThroughRouter` 覆盖公式、上游变化、缺失引用不猜绑、显式修复、Undo/Redo、重开及冷 BREP/Naming。浏览器 `associated next termination`、`surface termination picks` 分别验收数量操作与实际曲面拾取；各次执行结果以检查报告为准。

## 薄壁拉伸与切除

`PAD/LINEAR_EXTRUDE.thinWall` 在原草图之上派生精确截面，不向草图写入轮廓副本、额外变量或隐藏固定约束。`REGIONS` 消费整草图/已选区域的各条边界；`CHAINS` 显式保存普通草图曲线 stable IDs，允许开放/闭合及多个独立线链。连接来自已接受的正式 Coincident 端点图；同位置但无关系的端点不会被焊接，分叉和连接不一致明确失败。当前截面支持 Line、Arc、Circle；椭圆和样条偏置不是同类曲线，暂不支持薄壁。

厚度方式为 ONE_SIDED、TWO_SIDED、SYMMETRIC；对称方式的 thickness 为总厚度。第一厚度的主侧为 LEFT/RIGHT，第二厚度在相反侧；左右沿正式线链方向定义，reversed 明确翻转方向。thickness、thickness2 是正式长度 Quantity，PropertySlot 为 `thin:thickness`、`thin:thickness2`；后者仅双侧存在。MITER/ROUND 控制截面转角，开放端以平端闭合；不提供自动贴合既有材料的 Merge Ends。长度、对称/双侧轴向范围、关联终止和材料操作沿用直线生成器合同。

CHAINS 在显式创建/定义编辑时保存 `directions` 的曲线身份与方向标记。上游变化保留标记；新增段的 ID 排序不会改变已保存的厚度侧向。连接分裂或方向标记冲突时保留失败定义，可在面板明确“重设线链方向”，检查预览后确认。曲线丢失须重新选择，不能仅重存失效定义冒充修复。

Go 根据已求解曲线作解析偏置/连接，Worker 逐条薄壁校验精确区域，再由 OCCT 对各工具求并；覆盖区域保留材料，不使用全局环奇偶关系去除重叠。虚拟曲线、连接与端部以 Feature、命名侧及来源实体/端点组成稳定 ID；起始/末端不以坐标或遍历序号命名。`ProfileOrigin` 把派生区域/边/端点明确映射回正式草图来源，原生 Generated/Modified/Deleted 历史继续经过完整 Naming 门禁。零偏移一侧保留原曲线身份；普通圆柱改成内侧薄壁的外圆柱和外环端面引用已验收保留。

资源门禁为显式线链最多 256 个来源 ID、每条边界最多 512 条来源曲线、按最坏情况预估最多 4096 条生成曲线；在逐链构造之间检查取消。过厚导致半径非正、局部自交、相反连接、退化/无效实体及来源丢失均失败。多条墙体可以精确合并，不代表任意自交网络、所有曲线类型或所有厚度组合均已支持。

面板复用原拉伸编辑器，提供薄壁开关、来源/厚度方式、数量表达式、侧向、转角和方向重设；开放线链可在视区增减曲线或用曲线列表选择。失败提示可打开“修复薄壁输入”。Preview/取消不推进 Head，提交沿原 Domain Command、ChangeSet/CAS 与补偿历史。验收入口：`TestThinFeaturesThroughRouter`（圆环、同心墙覆盖、开放折线、圆接、公式、方向延长、关联切除、缺失修复、Undo/Redo/重开/冷 BREP/Naming）、`TestThinModeRetainsUnchangedSourceBoundaryThroughRouter`；浏览器 `thin extrusion selects an open chain` 覆盖真实拾取、公式、取消和历史。

## Sweep 路径与边界

`SWEEP` 以稳定草图实体集、方向锚点和显式反向保存路径，不持久化坐标匹配结果。所选曲线须由正式 Coincident/派生连接构成一条无分叉线链；源草图的支撑框架随参数重算。截面平面须包含路径起点并垂直于起始切向；不自动移动截面。可选 corrected Frenet 或 Frenet 标架，前者界面称修正标架，未提供额外扭转/缩放律。

光滑 G1 路径使用 OCCT Pipe 的真实路径边×截面边历史；直线尖角使用运输后的精确棱柱与切向平分平面裁剪并融合。混合曲线的非 G1 尖角需先加草图圆角；近回折、无效实体或自交拒绝。路径最多256曲线，路径×截面边不超过4096；不是工业规模容量保证。闭合路径无端盖；内孔和多区域保留精确构造。原生 `SweepSmoothMiterClosedAndAnnularHaveColdStableNaming` 与 Router `TestSweepFormalPathThroughRouter` 是几何/历史验证入口；浏览器 `Sweep creates from profile and viewport path` 验证实际选择、取消、提交和重开。

## 工程孔组、钻尖与标注

`holePoint` 表达单定位点，`holePoints` 表达1–128个共享孔参数的正式点集合，两者互斥；位置由各自普通草图显式点及支撑求值，成员身份来自草图/实体 ID。每个孔位须产生有效材料交集；不静默忽略实体以外的孔位。孔组整体提交一个 Feature，不把成员位置保存为下标。缺失成员保留定义并使 Feature 失败，用户逐项重选或明确移除。

盲孔可指定完整钻尖夹角。`CYLINDER` 深度指完整圆柱并额外增加钻尖；`TIP` 指含钻尖总深度，圆柱长度精确扣除 `r/tan(angle/2)`。贯穿和到平面不带有限钻尖。沉孔深度与螺纹标注深度须在完整圆柱范围内。钻尖通过精确圆锥构造和 Boolean 历史命名。

`thread.mode=ANNOTATION` 保存规格文字、左右旋、公称直径、螺距和标注深度，后三者有正式 Quantity/表达式及 ParameterId。实体仍按孔径生成，不生成螺旋牙型、不根据规格文字查表修改尺寸；请求几何螺纹明确报不支持。该标注随模型保存和重开，未承诺 STEP 标注交换或工程图出图。验证入口为 `EngineeringHoleGroupDrillDepthAndColdIdentity`、`TestEngineeringHoleGroupThroughRouter` 与 `hole_engineering_test.go`。

## 放样端部约束与连续性

自由放样保持既有 OCCT ThruSections 路径。`loftStart/loftEnd` 可指定垂直截面、稳定基准轴/草图直线方向或匹配支撑的 G1/G2，以及0.01–10的无量纲影响系数；系数有正式 ParameterId/表达式，改变条件会清理非活动参数。方向控制是端部导数，不代表沿整段插值的导向曲线。

有端部约束时，先保留已接受的截面对应，再把各截面曲线精确转换、升阶和插结到公共有理 B-spline 基底。各段使用三次 Hermite；任一端 G2 时使用五次，并将相应边界二阶横向导数设为零。验证所有齐次 U 基底系数的边界一/二阶导数关系，避免用少量法线采样冒充整条边界证明。面/端盖通过 Sewing 的实际历史组合命名；有效实体和自交检查均须通过。体积使用 OCCT 自适应积分，固定阶积分不足以验证这些高阶有理曲面。

G1/G2 当前承诺平面、圆柱和线性挤出面的同方向延续。每条截面边必须由精确 Common 全长度覆盖于至少一个所选有限支撑面，支撑方向必须是该解析面的挤出方向；不是任意自由曲面 G2。点端、直纹和过大的公共基底不进入该路径；最多65536控制点，检查点位于段间。支撑/方向失效保留定义并显式修复，同形面不自动替代。原生 `LoftEndJetsMatchExactCylindricalSupportsAndColdNaming`、Router `TestLoftBoundarySupportsThroughRouter`、浏览器 `Loft G2 picks stable cylindrical supports` 是对应验证入口。
