# 实体 Feature

返回[Part/Sketch](part-sketch.md)。当前行为以代码为准，范围和修复方式由本文维护。

## Feature 能力表

这是唯一 Feature 能力表；表述是代码合同，验证入口不表示当轮已执行。共享链为正式创建/编辑、Quantity/表达式与稳定参数、权威预览/取消、上游重算、抑制/删除保护、Undo/Redo 和重开。空白或不适用的参数不强行建立接口；导入基础实体的例外见表。

| Feature | 当前定义、编辑与修复 | 阵列／镜像来源 |
|---|---|---|
| SKETCH | 稳定实体/端点、正式约束与尺寸表达式；支撑重指定、投影删除、交点分割及引用影响检查，见 [Part/Sketch](part-sketch.md) | 空间草图框架与草图内派生成员；成员修改须解除关联 |
| PAD / LINEAR_EXTRUDE | 整草图或显式区域；有限、对称、双侧、贯穿切除及到平面；区域/终止面失效显式重选 | 有限工具；添加/净切除材料范围起点；关联终止仅实体阶段 |
| REVOLVE | 草图/区域、稳定轴、角度、反向；编辑轴与区域，失效不自动换区 | 有限工具；添加/净切除范围起点 |
| LOFT | 2–32 有序截面，至少一个单区域无孔闭合轮廓；首尾支持稳定点；编辑顺序、对应、接缝和相位；形状尺寸由截面参数管理，丢失输入需重选 | ADD/REMOVE 工具及相应范围起点，非任意截面重执行 |
| HOLE | 单个上游显式 POINT；普通/圆柱沉孔/锥沉孔，盲孔/单向贯穿/到平面；孔位、孔型与尺寸可编辑，丢点或终止面失效重选 | 三孔型 BLIND / 单向 THROUGH_ALL 切削工具；BLIND 可作净切除范围起点，到平面仅实体阶段 |
| FILLET / CHAMFER | 恒半径/等距，稳定边集；部分/全部丢失可逐项替换或移除 | 添加/净切除范围内修饰步骤，或明确实体阶段 |
| DRAFT | 稳定面集、上游中性平面、角度/反向；面和中性面显式修复 | 添加/净切除范围内修饰步骤；须通过对应包含检查，无独立工具 |
| SHELL | 开口面集、恒定厚度、内/外偏置；面集可修复，算法边界见[抽壳](shell-algorithm.md) | 仅实体阶段 |
| BOOLEAN | 同 Part 目标/工具阶段，并/差/交；编辑来源，单工具支持显式结果区域及修复；无尺寸参数 | 仅实体阶段 |
| SOLID_PATTERN | 多种子线性/圆周/镜像、稳定成员槽位与跳过；成员丢失后修复下游；数量/步距/角度表达式 | 有效新增成员工具，继承 ADD/REMOVE；数量1或全部新增槽位跳过时仅实体阶段 |
| SKETCH_PATTERN | 单草图空间框架复制，成员可供拉伸/旋转/放样；分布表达式与成员身份 | 草图框架，不是实体切削工具 |
| IMPORT_BODY | 冻结导入定义；正式命名建立及整体删除保护，无参数化几何编辑/任意抑制 | 仅实体阶段 |

依赖阻止非法删除/循环，抑制可能令后代 FAILED/BLOCKED；这不等于所有输入都能自动修复。实体特征保持 Feature/Parameter ID、名称、所属 Body 与历史位置，移动和插入限下述范围。基准与轴系是 Part 对象，不伪装为实体 Feature，约定见 [Part/Sketch](part-sketch.md#支撑与外部几何)。

## 定义与求值

实体生成、跨 Body 组合与局部修改复用 Feature、参数、ChangeSet、CAS、补偿历史及 per-Body BREP/Visual/Naming。算法成功后仍检查有效实体和完整命名，具体阶段见[求值链](part-feature-evaluation.md)。Body 不因材料不连通而改身份。

旋转以真实扫掠历史命名；轴上不属于最终结果的辅助边/点不进入输出。Loft 使用真实 OCCT AddVertex 点端和精确轮廓；自动比较循环起点、方向和圆相位后保存 stable seam、反向、相位与 correspondenceResolved，重算不重新猜对应。法向符号按截面位移统一，框架比较不移动输入。上限为64条轮廓边、一亿次候选比较；自交或构造失败拒绝。

圆角优先通用 OCCT，失败时可按稳定边顺序构造并合成真实历史；凸解析支撑与单条凹平面叶根的有界延伸/裁剪见 `kernel/occt/src/fillet_boundary.cpp`。保持半径、材料、连接及法向检查，不以提高容差接纳候选。凹根多边交会及与既有曲面相接不进入平面补充路径。最多128面、64条选边；不是任意大半径或任意角区保证。抽壳算法和历史仅由[抽壳算法](shell-algorithm.md)维护。

Boolean 并/差最多32工具，交集一个；工具定义保存 Body+Feature 阶段，BREP 和 Naming 属于同一快照。默认消耗工具输出，keepTools 可保留，抑制 Boolean 恢复输出；consumed 与 visible 独立，定义/历史不删除。


## 布尔结果区域

“全部结果”沿用既有布尔定义和多工具行为；省略 `booleanResult` 也表示全部。显式模式限定同 Part 一个目标 Body、一个工具 Body 的合法闭合实体阶段。首次切到“选择区域”选中当前候选；确认后保存明确集合，后来新增的区域不会自动加入。`keepTools` 只控制工具输出，不决定保留哪些结果区域。

`AnalyzeBooleanRegions` 独立重建上游阶段的配对 BREP/Naming，再由 OCCT CellsBuilder 在最终融合之前分割出独立体积。并／差／交限定候选归属，同归属的独立体积仍可分别选取。普通结果为空或没有材料变化不阻断分区分析。精确分区按输入及来源命名缓存；改变保留状态仅组装已分割的体积并清理内部／同域边界，允许同一 Body 包含不连通实体。空选择、空／无效实体和材料未改变的最终候选不能确认。

一次区域分析冻结结果、资源清单与范围摘要，下载在校验文档权限、当前 Revision、目标/工具阶段、分析所有者及资源归属后直接读取，不再重新分区或发布制品。清单复用有界预览缓存，五分钟有效；过期或 Head 改变后需重新分析。

区域引用保存有向边界的来源拓扑证明及冻结输入／体积摘要，不保存显示编号、数组位置或 mesh ID。尺寸变化后只接纳唯一来源对应；来源证明本身无法区分的区域，仅在输入完全相同时用冻结身份区分。消失、拆分、合并或歧义保留原定义并要求重选／移除，不猜选、不回退全部。显式集合按当前解析出的区域去重，两项失效输入替换为同一区域仅保留一项；其他未修复输入仍保留，未处理前不能确认。最终组装进入既有 Generated/Modified/Deleted、未变拓扑证明及完整 Naming 门禁，预览、提交、依赖、缓存、ChangeSet 和 Undo/Redo 仍走原链路。

对话框复用紧凑选择列表和选择会话；点击候选表面操作整个体积，保留／排除项可反复增减、定位或临时单独显示。候选在最终预览计算中继续显示；颜色、透明度、轮廓和列表状态表达角色。关闭、取消和过期响应清理临时显示，不写正式可见性或历史。区域分析制品无可绑定的正式 Naming，不能作为后续特征的输入阶段。详细引用和分析信息进入 DEBUG。

当前上限为 128 个分割体积；不支持多工具显式选区、开放壳或 mesh 分区，复杂来源无法唯一对应时需要人工重选。源码入口：`workspace/boolean_regions.go`、`OcctKernel::analyze_boolean_regions`、`boolean-feature-dialog.tsx`；回归入口：`BooleanCells*` / `BooleanCellAssembly*`、`TestBooleanResultRegionsLifecycleThroughRouter` 和 `solid-feature-live.spec.ts` 的区域拾取／修复场景。

## 调整特征顺序

结构树的实体 Feature 对象菜单提供“调整特征顺序”：指定同 Body 锚点及之前/之后，权威预览通过后确认。普通生成、Hole、圆角/倒角/抽壳/拔模可以移动；导入、Body 创建者/首个基础实体、Boolean、实体阵列/镜像是同 Body 的固定边界，不能移动或跨越。已有 `FEATURE_DELTA` 范围的有序成员集合保持不变。草图不自动随特征移动。

正式 `MOVE_FEATURE` 保存源/锚点 Feature ID、定位及期望定义摘要；确定性地从全局 `model.Features` 取出单项再插入，其他项相对顺序不变，并同步全部 `Order`。每项受影响定义在同一 ChangeSet 内补偿，Feature/Parameter ID、名称、Body、参数及显式引用不变。取消和预览不写 Head，输入或 Revision 变化不能复用旧候选；提交沿原幂等、CAS 和 Undo/Redo 链。

按候选顺序重建 Body 前驱和依赖图，检查全 Part 的轮廓、孔位、支撑、终止面、外部投影及拓扑输入。语义 Anchor 和实际来源阶段分别验证；未显式保存阶段的既有引用从其不可变来源 Revision 的精确 Body Tip 核对，不补写或重绑引用。所有活动引用和 Body 的实际几何必须成功，失效指明阻塞特征并原子拒绝，不接纳 FAILED 移动。顺序与候选前驱进入原阶段输入、缓存和增量求值。

入口：`services/internal/workspace/feature_move.go`、`feature_insert.go`；真实 Router/Worker 回归 `feature_move_integration_test.go`、`feature_insert_integration_test.go`，浏览器 `solid-feature-live.spec.ts`。

## 在实体阶段后插入

结构树实体步骤的对象菜单“在此步骤后插入”仅开放圆角/倒角，复用原面板并持续显示 Body、锚点与插入位置。上下文只存在于该次面板；取消、提交、Revision 或编辑目标改变后退出，不建立持久历史游标。锚点必须是当前 Revision 中指定 Body 的有效、未抑制实体阶段；最终 Body 失败时也不借 displayFallback 授权。

`feature-input` 的独立 `afterFeatureId + bodyId + versionId` 模式重建锚点精确前缀，显示制品下载也按该上下文核对；边拾取必须匹配 GeometryKey，并返回 PersistentSelection、来源 Revision 及实际 SourceFeatureID。创建端再次验证阶段和 creation evidence。该模式与编辑输入、只读 `ResultStage` 不混用，不能绑定下游、最终结果或其他 Body 的几何。

原 `CREATE_MODIFY_FEATURE` 携带临时插入信息，在内存候选中直接插入并同步全局数组/Order；按稳定 ID 读取新定义。新 Feature、参数与后续 Order 变化进入同一 ChangeSet/Transaction，沿既有预览、幂等、CAS、补偿历史与缓存提交。原定义和参数身份不变。插入和移动复用候选依赖、实际来源阶段、全 Part 引用/几何检查；下游或跨 Body 失败指出阻塞对象并原子拒绝。固定节点保持相对顺序，允许以其合法输出为锚点，不套用移动的固定边界规则；已有 FEATURE_DELTA 的有序成员集合不能改变。

入口为 `services/internal/workspace/feature_insert.go`、既有 `applyCreateFeature` 与 `solid-feature-editor.tsx`。定向回归：`services/internal/control/feature_insert_integration_test.go` 和浏览器 `solid-feature-live.spec.ts` 的 `insert fillet after an exact base stage`；覆盖阶段拾取、底座→新圆角→已有倒角、取消/过期、一次撤销恢复全部顺序、Redo、重开/冷重建及下游/跨 Body/范围保护。仅此两种局部特征，不包含草图、生成特征插入或跨 Body 创建；主体实机与全部参数组合仍需单独验收。

## 独立 Hole

`HOLE` 是一个独立 Feature，支持普通孔、圆柱沉孔与锥沉孔，复用创建/编辑、Preview/Commit、ChangeSet 和历史链。定位保存同 Part 合法上游普通 Sketch 的单个显式 POINT 稳定引用，轴沿已求解支撑的有向法向，可反向；不保存视口坐标，不生成隐藏圆草图或额外切除 Feature。孔位通过原草图尺寸/约束修改，多点草图必须明确选点；端点、圆心、原点与阵列派生点不属于定位输入。

三种孔都管理主孔直径，盲孔还管理从定位平面起算的平底总深度；单向贯穿只切除该平面沿选定方向的目标 Body 材料，不改变普通拉伸的贯穿语义。圆柱沉孔增加沉孔直径和沉孔深度；锥沉孔增加沉孔直径和**完整锥面夹角**（0° < α < 180°），锥段深度为 `(沉孔直径 − 主孔直径) / (2 tan(α/2))`，面板显示派生值。沉孔直径必须大于主孔直径，沉孔深度必须为正，盲孔沉孔段必须短于总深度；非法组合、无材料交集和无效结果拒绝。

所有活动尺寸沿用 Quantity、单位、表达式、依赖和 managed ParameterID；孔型/终止模式切换移除非活动参数，共有参数身份及自定义别名保留，Undo/Redo 恢复原定义。evaluator 构造一个精确切削工具，由主圆柱与原生圆柱/圆锥通过真实 OCCT 历史融合，再进入既有 Boolean/Naming。主孔壁、沉孔壁、台阶、锥面、孔口和孔底按实际构造与裁剪命名，保留定位草图与点的完整来源身份；确定的尺寸修改延续引用，孔型切换后消失的角色保留失败定义，要求明确重选，不自动换面。Hole 编辑若仅下游失败，预览显示权威孔阶段并指出实际待修复特征，提交仍完整求值；孔自身或上游失败不允许确认。

定位草图、点实体和目标上游阶段进入依赖、删除/循环检查及缓存；合法删点留下 `HOLE_POINT_MISSING`，下游按依赖阻塞。编辑器保留待修复输入并提供“修复孔位”；草图支撑失效沿[已有支撑重指定](part-sketch.md#支撑与外部几何)修复。取消和预览不写 Head，修复保持 Hole ID、名称、Body 与历史位置。同 Body 的合法 `BLIND` / `THROUGH_ALL` 三种孔型可作为 `GENERATOR_TOOL`，执行线性／圆周阵列与镜像，详见[参数化阵列](#参数化阵列)。单向贯穿按成员重建主孔切削范围，入口沉孔保留尺寸；有限 BLIND 可与后续修饰组成净切除 `FEATURE_DELTA`；显式 `BODY_STAGE` 保持复制整个已求值阶段的原意。到平面 Hole 不作为复制工具；尚无钻尖、螺纹、多点孔组。

源码入口：`workspace/hole.go`、`evaluation_projection.go`、`occt_kernel.cpp` 的 `add_hole_recess` 与 `solid-feature-editor.tsx`。定向回归入口：`GeometryExchange.Hole*`、`TestEngineeringHole*`、`TestHoleLifecycleThroughRouter`、`TestHoleToPlane*` 和 `solid-feature-live.spec.ts` 的 Hole 场景；覆盖精确体积/方向、表达式、引用延续/失效修复、抽壳外壁支撑、历史与冷重建。执行结果以当轮交付为准；当前环境是 SQLite、Release OCCT 7.9.1 与 Chromium/SwiftShader，不代表 PostgreSQL 或全部几何组合验收。

## 到平面关联终止

单方向直线拉伸／切除及 Hole 的 `TO_PLANE` 目标为标准/自定义基准面，或同 Part 合法上游实体平面的无限延伸支撑平面，包含已有跨 Body 场景。显式区域集合、外环和孔沿用精确 Profile Builder；目标不受有限面的边界限制。定义保存 `terminationPlaneId` 或 `terminationPlane`（PersistentSelection、来源 Revision、Body 与实际来源阶段），不以语义 Anchor 代替阶段，也不保存一次测得的长度作为终止条件。

Hole 主孔从定位草图平面沿所选法向终止，入口圆柱沉孔/锥沉孔保留尺寸；不保存测得孔深，也不创建深度参数。完整精确入口工具必须严格位于终止面之前，平面截入沉孔段或较宽孔口也拒绝。外部目标先经 Publication → ContextInput → 本地基准面接入，不直接绑定外部实体面。

每次求值从当前合法上游前缀解析目标框架；Proto/Worker 接收当前精确平面，OCCT 以棱柱和支撑半空间求交形成平行或倾斜端面。每个选中区域的所有点沿指定方向都须有正终止距离；平行、反向、穿过起始轮廓、零厚度或无效结果明确失败。端面、侧面与边/点使用真实生成及裁剪历史进入原 Naming 门禁。目标移动或旋转进入依赖和缓存，删除、循环及非平面目标由权威校验拒绝。

面板明确显示“到平面”，可拾取和重选基准面/实体平面。失效目标保留原定义及解析状态，失败提示提供“修复终止平面”入口；候选来自当前特征之前的合法阶段，不能绑定下游或 displayFallback。显式修复须解析全部必需输入，沿原 Preview/EDIT_FEATURE、ChangeSet 与历史提交，保持 Feature ID、名称、Body 和位置。合法上游变化可留下可检查的 FAILED/BLOCKED；服务、权限、制品错误继续返回实际错误。圆边变为椭圆等几何类型变化仍按现有引用合同要求明确重选。

`GENERATOR_TOOL` 和 `FEATURE_DELTA` 不接受此终止模式，线性/圆周阵列及镜像共用此能力限制。`BODY_STAGE` 仍可刚体复制已经求值的实体阶段，不在每个成员位置重新终止。未包含到下一面、有限面边界、任意曲面、双向关联、终止偏移。

定向入口：`services/internal/control/to_plane_integration_test.go` 的真实 Router/Worker 生命周期、目标失效与修复、历史/冷重建；`GeometryExchange.ToPlane*` 的精确体积、内孔、多区域、正反方向、Naming 与非法距离；`solid-feature-live.spec.ts` 的 `to-plane` 实际拾取和修复。

## 参数化阵列

`CREATE_PATTERN` 创建 `SOLID_PATTERN` 或 `SKETCH_PATTERN`；编辑、预览、抑制、删除、补偿历史和 per-Body 制品沿用 Feature 链。共同定义为稳定源引用、`PatternDefinition` 分布及逻辑槽位；实体多种子在 `additionalSources` 保存其余各项的正式阶段／工具／材料范围定义，逐项进入依赖、删除及范围保护、缓存和 Worker，作为同一 Feature 一次预览／提交／历史操作。种子限定同 Body、同添加／切除语义，最多32项；不以首个种子代替其余输入。空间草图阵列仍一次引用一个草图，草图内几何可多选。控制面只计算刚体变换，三类几何分别求值。长度、角度、相位、数量使用既有 Quantity、ParameterID 和 checkedAST，不由浏览器逐成员发命令。

数量包含槽位 0，求值后必须是 1–256 的整数。线性支持固定步距和总跨度；圆周另支持整圆均布（步角为 360/N，不生成重复终点）。数量为 1 不除以零。跳过成员不重新编号；暂时超出当前数量的跳过槽位保留，恢复数量后仍跳过。成员身份来自 PatternId、槽位、源身份；位置和显示顺序不参与身份。成员消失后的下游引用明确失败，不按相近几何改绑。编辑实体阵列／镜像时，仅下游失败且目标和上游阶段成功的候选可预览有效目标阶段，显示待修复特征；提交仍完整求值并保存真实 FAILED/BLOCKED，随后由原下游编辑器明确重选。当前操作或上游无效不能确认。

- `GENERATOR_TOOL` 复用同 Body 明确上游拉伸/旋转/放样生成工具、盲孔及单向贯穿普通孔／圆柱沉孔／锥沉孔的切削工具及已求值实体阵列／镜像工具。能力按有效新增成员判断：数量为1或所有额外槽位均跳过时不可作为工具，恢复成员后重新可用；继续复制阵列时只复制其新增成员工具，排除槽位 0，继承添加／切除语义与真实历史。切除阵列构造并变换切削工具，再与当前上游 Body 做差，不复制已经打孔的主体。有限、对称、双侧拉伸及有限旋转可复用；普通拉伸贯穿、到平面及通用逐位置特征重执行不支持。
- `FEATURE_DELTA` 保存稳定 `startFeatureId` 与结束 `source.featureId`。起点限同 Body 有限拉伸（含对称/双侧）、旋转、放样及三种 BLIND Hole，后续只能是连续的 FILLET/CHAMFER/DRAFT。设 B0 为起点前 Body，B1 为末端 Body：ADD 提取 `B1\B0` 并验证 `B0\B1` 在既有容差内为空；REMOVE 提取 `C=B0\B1` 并验证 `B1\B0` 为空。工具须非空且为有效实体集合，包含检查来自真实差集，不比较两者总体积。允许内圆角局部加回材料，只检查最终为合法净切除；正反拔模均按实际几何判断。范围不是通用重执行，THROUGH_ALL/TO_PLANE、Boolean、Shell 和 Pattern 不作起点。AUTO 从末端修饰识别范围，种子行显示起止名称；全部接受引用进入依赖、删除和历史范围保护。
- `BODY_STAGE` 复用指定上游实体阶段。`COMBINE` 使用现有布尔；添加材料可选择 `INDEPENDENT`，在同一 Body 保留独立 Solid，不按数量创建 Body/Part。槽位 0 是已存在的上游种子，不能跳过或由阵列相位搬移；实体阵列的相位应设在种子参数上。草图和空间草图阵列可直接设置相位、跳过槽位 0。
- `SKETCH_FRAME` 复用已求值二维草图与变换后框架，正式 Profile 引用保存阵列 FeatureId 和 `profileMemberSlot`；放样截面分别保存 `memberSlot`。同一阵列的多个成员可作为不同截面，不新增三维草图求解器或文档。每个槽位在树与同一 GLB 快照中拥有独立的关联草图身份和显示组；视图区或树选择成员后，下游命令写入明确槽位，不能把整个阵列当作单个轮廓。源 EntityId 保留在成员命名空间内，供截面接缝引用。

`DocumentView.replicationSources` 由领域模型派生工具、材料范围起点/修饰步骤与实体阶段能力及推荐来源，前端拾取和列表共享此读模型；命令可传临时 `AUTO` 意图，由控制面按精确上游前缀解析并持久化既有明确来源类型、范围和运算。已接受定义不随能力变化自动改义，显式 `BODY_STAGE` 仍复制整个阶段，不能把它当成切削工具。创建/编辑仍由领域校验及真实 Worker 验证。多个显式种子中，材料范围已包含的生成工具或较早同起点范围仅保留为正式输入、依赖和删除保护，求值复制最大已选范围，添加范围避免重复工具重叠；净切除范围避免原始刀具再次切掉内圆角保留材料。包含关系只按精确历史 ID 判断；独立种子及显式整个实体阶段不被改义。新增操作先实现独立工具与完整 Naming，再声明来源能力。圆角、倒角等局部修改不冒充独立工具；切除与整体阶段不能混用。复制成员拾取不自动跳回种子。

实体镜像保存为同一 `SOLID_PATTERN` 的 `MIRROR` 分布，固定保留槽位 0 并生成槽位 1；引用标准/基准面或上游实体平面，反射矩阵由实时引用求值，不以界面中的缓存方向为真相。实体平面复用拔模的持久面解析与 feature-input 上游显示，进入依赖和删除保护。旋转与反射共用正交等距变换、工具来源、组合运算及成员 Naming；无新增持久命名或几何求解架构。仅实现实体特征镜像，不新增草图阵列镜像或装配镜像。

内核阶段检查点留存可复用的生成工具、实体阶段与材料范围基线，供后续追加阵列续算。每个合法范围起点在尚无消费者时也保存私有 Shape、Naming 和完整性状态；净切除工具从两侧真实布尔历史命名，提取和复制使用私有副本，避免污染检查点。盲孔保留沉孔构造后的完整工具及 Naming；单向贯穿孔同时保留原孔型、起点、有向轴、入口尺寸及稳定的成员变换/命名链，即使当时没有复制消费者也进入检查点。求值和阶段缓存版本隔离旧检查点。

单向贯穿成员按完整变换链在同一个冻结的阵列前 Body 中重建：以精确 BREP 在成员轴向框架的界限确定足够主孔长度，只加容差余量，起点不后移，不复制种子阶段的有限长度或使用固定超长工具。沉孔入口随成员变换，尺寸不随厚度改变、不自动贴面。混合种子分别保留有限工具和贯穿定义，二次复制组合变换且只继承新增成员、排除槽位 0；若整个目标均位于成员起点后方则明确拒绝。贯穿复制在私有目标/工具副本上执行布尔，避免取消预览或相反孔轴的相交运算污染共享检查点；真实复制与布尔历史共同进入 Naming。

复用种子几何，以 `BRepBuilderAPI_Transform` 真实历史合成 Boolean/同域合并 Naming。拓扑预算为源命名输出数乘成员数不超过 100000；成员身份稳定不保证布尔后的面永不分裂，仍执行完整性、缺失与歧义门禁。任意特征组、拉伸贯穿／到平面逐成员重新终止、路径阵列、逐成员变尺寸、任意局部特征重执行未包含；这些对象仍可在合法时明确选取已求值实体阶段。

入口：Part 和草图工具栏的“阵列”（面板内切换线性／圆周），Part 工具栏的“镜像”；空间草图成员可在视图区和树中分别选择，用于独立拉伸、旋转和放样。实体与空间草图阵列在树中编辑/抑制/删除；草图内阵列也有独立树节点，解除关联仍由阵列面板执行。空间与实体阵列默认绑定标准轴，方向、轴和圆周中心支持视图区拾取，反向改变分布步进符号。中心和方向的源引用进入依赖图及删除保护。

定向入口：`GeometryExchange.BlindEngineeringHoleToolsReplicateFromWarmCheckpoints`、`GeometryExchange.DirectionalThroughHoleToolsRebuildAgainstFrozenPatternTarget`、`GeometryExchange.FiniteRemovalRangesPreserveInnerFilletsAndWarmNaming`、`draft_pattern_integration_test.go` 的有限加工范围场景、`services/internal/control/hole_pattern_integration_test.go` 与 `solid-feature-live.spec.ts` 的 `finite Hole` 场景，分别检查完整沉孔工具体积／成员 Naming、真实命令历史与温缓存／冷重建、视图区与结构树拾取及成员失效后的原编辑器修复。运行结果以当轮交付为准。

## 阶段、命名与历史

依赖连接上游 Feature 输出，而不连接 Body 的未来最终结果。局部修改先重建自身之前的 Body 前缀，再以保存的 source Revision、Body 和 PersistentSelection 解析当前输入。缺失、歧义或类型变化必须失败，不能用最近几何或持久化 local ID 恢复。显示层的 local pick 只用于创建正式持久选择。

拔模所选角度面的单一 Modified 输出以 DRAFTED_FROM 归属拔模，并沿原面保持 Modified 延续；未变面/边不改归属。真实分裂/合并仍走原规则，不猜测面对应。生成历史、Boolean、same-domain unify 和局部修改的真实 Generated/Modified/Deleted/存活关系形成同一命名链。抽壳同时保留外面并生成内面时保留两类来源；多对多来源不能生成重复 semantic ref。未被同类型历史覆盖的边/点继续使用已有语义邻接闭包，完整门禁覆盖每个 Face、Edge、Vertex。控制面 evaluator 以[求值入口](../../../services/internal/workspace/service.go)为准，Naming evaluator、策略 ID、digest 与容差以[命名策略源码](../../../services/internal/modelcore/topology_naming.go)为准；缓存包含版本和完整来源历史。

精确边的直线识别由同一个内核判定供类型查询、方向属性与 Naming evidence 使用。非周期 B-spline 在实际裁剪参数区间内，正权控制点全部落在端点直线的 `Precision::Confusion()`（1e-7 mm）容差内，且沿直线单调排列时，识别为 `LINE`；不依据显示折线或有限采样。原 BREP 和真实 History 保留，属性同时报告 `underlyingCurveType=BSPLINE_CURVE` 与原生参数。选择 evidence 使用解析直线原点、单位方向和端点距离参数，现有轴/方向解析及外部几何投影消费同一证据，不能把 0–1 的样条参数当作毫米。弯曲、回折或周期样条保守保留曲线类型。evaluator 版本进入缓存键；旧制品须经正常特征编辑/重新求值生成新的成套 Naming/Visual，再选择其边。

Feature 定义编辑保持稳定 ID、顺序和名称；生成和局部修改保持所属 Body。参数 Source 的更新与新增/移除和定义共用一次 ChangeSet。结构树可抑制/解除抑制所有建模实体 Feature，专用 `SET_FEATURE_SUPPRESSION` 命令检查 definition digest 并只改变显式 suppressed 字段，沿用 entity ChangeSet 和 Undo/Redo；导入基础实体保持受保护。失败状态与诊断为派生值，不进入可补偿的 Feature 定义。

## 用户操作与编辑上下文

所有实体 Feature 面板共用视图区输入规则。几何对象不作为 UUID 下拉选项：轮廓、旋转轴、中性平面、边/面集、布尔目标/工具、放样截面和闭合起始边均直接拾取。运算、范围、直纹/平滑是参数选项，使用分段按钮。命令可以先选对象再启动，也可以先启动再选对象。

- 面板中的输入按钮切换当前拾取角色。角色先过滤原始 hit，再按业务对象投影；拉伸／旋转的显式轮廓保留权威区域 ID，整草图及放样截面选整个草图；旋转轴和闭合起始边保留草图的单条实体，边/面不能被邻近顶点抢走。所有输入限定同一所有者文档、Revision 和 occurrence。基准面/轴输入临时显示该 occurrence 的参考几何，退出恢复原显示偏好；保存选择的高亮携带已提交编辑会话的完整 InstancePath 和 contextVariantKey。
- 进入实体 Feature 命令不改变相机 fit。旋转拾轴阶段的已选轮廓只作淡色上下文，轴线选择和 hover 分别使用更明显的颜色和线宽，包含轮廓自身的直线。
- 圆角/倒角自动带入已选边，拔模/抽壳自动带入已选面。后续点击累加，再次点击取消，不要求 Ctrl 或“添加当前选择”。多选集合默认显示紧凑摘要，可展开条目定位、更换、移除；有序截面保留顺序操作。高亮显示真实几何。异步持久绑定顺序执行，不能因为快速连续点击丢掉输入。
- 放样截面按拾取顺序添加；“点端”拾取保留具体点引用，可上移至首端。直接选择仅含一个点的普通草图也会在预览/提交前绑定到该稳定点引用；多点草图要求明确选点，已接受轮廓不会因上游变成点草图而自动改类。默认自动对应，可逐截面重新自动、反向、拾取闭合边或设置圆相位；可选连接线来自当前成功预览的权威边界点，随预览失效清除。编辑控件显示已接受自动对应，人工修改以它为基线。
- 输入完整或参数修改后，250 ms 合并请求既有权威预览命令；新输入中止旧请求，旧 generation/Revision 的结果不能显示或提交。仅当前输入获得成功预览后启用提交；预览失败移除旧候选，修正输入自动恢复。取消、文档/Revision/occurrence 变化结束会话。预览显示结果和上游参考线，同时仍允许继续拾取原始输入。
- 编辑已有生成、放样、实体阵列和局部特征时通过只读 `POST /api/documents/{id}/feature-input` 重建自身之前的 Body 阶段并解析原选择，高亮显示原边面。该接口也为阶段上的新点击绑定持久选择；它不推进 Head。新绑定使用 `sourceVersionId + sourceFeatureId` 指明不可变来源，重算仍由真实 Naming 解析，不保存局部序号。已保存的旧 Revision 选择继续按既有最终输出来源解析。
- 编辑布尔恢复目标上游阶段和已消耗工具的明确输出阶段；工具的临时可见性不修改持久 `visible/consumed`。输入阶段制品下载按文档权限、当前 Revision、Feature 和精确对象范围授权。

## 失败和预览

创建失败与预览失败原子拒绝。已有 Feature、参数或草图编辑引起的可识别实体求值失败可以提交 FAILED Revision：失败 Feature 为 FAILED，依赖后代为 BLOCKED，受影响 Body 不携带旧成功 GeometryKey。参数/定义修复或 Undo 重新求值；Redo 可恢复失败定义，显示元数据编辑也不能把 FAILED Revision 标为成功。预览不推进 Head，变更输入或 Revision 后不能采用旧候选。

失败 Body 可携带派生 `displayFallback{geometryKey,sourceVersionId}`，指向该 Body 最近成功修订的显示结果；连续失败或显示元数据编辑保持同一成功来源，恢复成功立即清除。Part/Product 视口以半透明红褐色显示旧 Mesh，树和视口提示 FAILED/BLOCKED；显示遵循 Body/occurrence 可见性。旧实体可用于相机导航，不能参与建模拾取。快照仅开放其 Visual 制品，BREP/Naming 下载、持久绑定和导出仍要求当前成功结果；fallback 不进入可补偿 Body 定义。重复投影为只含 Visual 的读模型时，继续保留原 `visualNamingDigest`，保证重开和缩略图仍可验证 GLB 的来源关联。

结构或参数验证错误仍拒绝候选；草图支撑及外部几何按[各自的失败与重指定合同](part-sketch.md#支撑与外部几何)处理。这里没有把所有数据库/网络错误归为可持久化的模型失败。

## 输入修复边界

`feature_input.go:GetFeatureInput` 重建目标特征之前的精确上游阶段，再逐项解析原边/面及中性面。`picks` 保留原索引和每项状态：RESOLVED、MISSING、AMBIGUOUS、TYPE_MISMATCH、OUTSIDE_CURRENT_TIP、SOURCE_UNAVAILABLE 或 CONTRACT_MISMATCH；只有唯一解析的项带 localId。实体中性面还要求精确 PLANE。跨 Body 来源被抑制为空时保留目标几何并报告中性面缺失；上游自身求值失败则返回实际失败 Feature ID/名称及原错误，不转换为引用状态。权限、服务和制品读取错误仍沿原错误链返回。

输入阶段的制品下载通过 `GetFeatureInputGeometry` 独立重建合法几何，再核对文档、Revision、Feature、Body 与对象范围，不依赖旧引用是否解析成功。Boolean 工具仍限定保存的上游工具阶段，历史结果仍只读；不能用下游结果或 displayFallback 绑定输入。

`solid-feature-editor.tsx` 保留全部原定义，仅对有效项高亮。失效项必须明确替换或移除；未处理项、空的必需边/面集、未指定有效中性面都禁止预览和确认。新选择从同一只读阶段绑定正式 `sourceVersionId + sourceFeatureId + PersistentSelection`，不自动采用歧义候选或最近几何。保留的有效输入不重新绑定。

沿用权威 Preview 和 EDIT_FEATURE；显式局部特征编辑在接纳前校验全部必需输入，不能把未处理引用保存为“修复成功”的 FAILED Revision。原 Feature ID、名称、顺序及 Body 保持不变；上游/参数变化导致的 FAILED/BLOCKED 与合法引用下的几何算法失败仍沿用原合同。预览、取消不写 Head；输入变化使旧候选失效。

## 验证入口与限制

- `services/internal/workspace/solid_feature.go`、`replication_source.go`、`pattern.go`：定义、参数、能力、正式来源和范围去重；`feature_input.go`：逐项引用修复。
- `kernel/occt/tests/geometry_exchange_scenarios.cpp`：真实精确几何、材料包含、拓扑历史和热/冷检查点；`DraftAdditiveRanges*` 覆盖添加型拉伸/放样/旋转、正反拔模及三种复制。
- `services/internal/control/*_integration_test.go`：solid_feature、feature_input_repair、loft、hole、hole_pattern、draft_pattern、replication_loft_mirror、to_plane、boolean_regions、feature_move/insert 对应真实 Router/Worker 链；`solid-feature-live.spec.ts` 验证视图区/树拾取、预览取消、编辑修复与历史。
- `mise run check --scope geometry`、`--scope workspace`、`--scope services`、`--scope web`；运行与共享资源见[测试入口](../../../tests/README.md)。测试存在、定向通过、浏览器验收分别举证。

已知限制：不支持 Sweep、到下一面/任意曲面终止、范围以外的切除组合、任意历史插入、跨 Body 移动和任意特征重执行。复杂拓扑分裂/合并可能必须显式重选。常规建模与正式历史链已接通，不代表所有曲面/参数组合或所有 Feature×复制×修复组合完成验收；未运行组合属于验证缺口，不写成保证。
