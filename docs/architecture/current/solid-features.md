# 实体 Feature

返回[Part/Sketch](part-sketch.md)。当前行为以代码为准，范围和修复方式由本文维护。

按[整体设计流程](part-design-workflow.md)进入建模；生成算法见[实体生成器](solid-generators.md)，成员复用和重执行见[特征复制](feature-replication.md)。本页保留唯一能力表、历史位置和通用编辑/修复合同。

## Feature 能力表

这是实体 Feature 能力表；曲线/曲面范围归[线框与曲面](wireframe-surfaces.md)。表述是代码合同，验证入口不表示当轮已执行。共享链为正式创建/编辑、Quantity/表达式与稳定参数、权威预览/取消、上游重算、抑制/删除保护、Undo/Redo 和重开。空白或不适用的参数不强行建立接口；导入基础实体的例外见表。

| Feature | 当前定义、编辑与修复 | 阵列／镜像来源 |
|---|---|---|
| SKETCH | 支持独立 Part／直接 READY ContextVariant 历史补建，见[历史位置创建](#在历史位置创建)；稳定实体/端点、正式约束与尺寸表达式；支撑重指定、投影删除、交点分割及引用影响检查，见 [Part/Sketch](part-sketch.md) | 空间草图框架与草图内派生成员；成员修改须解除关联 |
| PAD / LINEAR_EXTRUDE | 独立 Part／直接 READY ContextVariant 支持合法上游位置历史创建；整草图或显式区域；有限、对称、双侧、贯穿切除、到平面/下一面/曲面及起始/终止偏移；开放线链/闭合边界薄壁；区域/终止/线链失效显式重选 | 有限工具、添加/净切除材料范围起点及生成器成员重执行；关联终止逐位置重新计算 |
| REVOLVE | 草图/区域、稳定轴、角度、反向；编辑轴与区域，失效不自动换区 | 有限工具、添加/净切除范围起点及生成器成员重执行 |
| LOFT | 2–32有序截面；自由放样支持点端、对应、接缝和相位；端部法向/方向及解析挤出支撑的 G1/G2，影响系数有参数/公式 | ADD/REMOVE 工具、范围起点和生成器成员重执行；连续性支撑仍须逐位置满足 |
| SWEEP | 正式草图单路径或[三维几何曲线](wireframe-surfaces.md)；闭合截面及内孔、两种标架、方向；平面直线斜接与空间 G1 路径范围分别校验；独立实体/加料/切除/交集，路径缺失显式修复 | 有限工具、材料范围起点及既有生成器成员重执行范围 |
| THICKEN | 独立 Part 的有界 Sheet、有符号常厚度、正式结果/面引用；进入原 Body 材料操作与修改链，见[线框与曲面](wireframe-surfaces.md) | 实体阶段；未扩展加厚成员重执行 |
| HOLE | 单点或多点共享参数；普通/圆柱沉孔/锥沉孔，盲孔/单向贯穿/到平面；可选钻尖及圆柱/总深度定义；螺纹工程标注，逐点修复 | 三孔型 BLIND / 单向 THROUGH_ALL 切削工具；BLIND 可作净切除范围起点；含到平面的合法孔可选成员重执行 |
| FILLET / CHAMFER | 恒半径/等距，稳定边集；支持历史阶段插入，部分/全部丢失可逐项替换或移除 | 添加/净切除范围内修饰步骤，或明确实体阶段 |
| DRAFT | 稳定面集、上游中性平面、角度/反向；支持历史阶段插入，面和中性面显式修复 | 添加/净切除范围内修饰步骤；须通过对应包含检查，无独立工具 |
| SHELL | 开口面集、恒定厚度、内/外偏置；支持历史阶段插入和面集修复，算法边界见[抽壳](shell-algorithm.md) | 仅实体阶段 |
| BOOLEAN | 同 Part 目标/工具阶段，并/差/交；编辑来源，单工具支持显式结果区域及修复；无尺寸参数 | 仅实体阶段 |
| SOLID_PATTERN | 多种子线性/圆周/镜像、稳定槽位与跳过；可复制工具/阶段/材料范围，或在成员位置重执行五类生成器；数量/步距/角度表达式 | 有效新增成员工具，继承 ADD/REMOVE；数量1或全部新增槽位跳过时仅实体阶段 |
| SKETCH_PATTERN | 单草图空间框架复制，成员可供拉伸/旋转/放样；分布表达式与成员身份 | 草图框架，不是实体切削工具 |
| IMPORT_BODY | 冻结导入定义；正式命名建立及整体删除保护，无参数化几何编辑/任意抑制 | 仅实体阶段 |

依赖阻止非法删除/循环，抑制可能令后代 FAILED/BLOCKED；这不等于所有输入都能自动修复。实体特征保持 Feature/Parameter ID、名称、所属 Body 与历史位置，移动和插入限下述范围。基准与轴系是 Part 对象，不伪装为实体 Feature，约定见 [Part/Sketch](part-sketch.md#支撑与外部几何)。

## 定义与求值

实体生成、跨 Body 组合与局部修改复用 Feature、参数、ChangeSet、CAS、补偿历史及 per-Body BREP/Visual/Naming。算法成功后仍检查有效实体和完整命名，具体阶段见[求值链](part-feature-evaluation.md)。Body 不因材料不连通而改身份。

旋转、Sweep 和自由/约束放样的精确构造、对应及连续性边界见[实体生成器](solid-generators.md)。生成结果仍经过统一有效实体与完整 Naming 门禁。

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

结构树支持选择同 Body 的单个或连续实体 Feature 后“调整特征顺序”：连续性按该 Body 的实体历史判断，包含区间内抑制项；指定集合外同 Body 实体锚点及之前/之后，权威预览通过后确认。普通生成、Hole、圆角/倒角/抽壳/拔模可以移动；导入、Body 创建者/首个基础实体、Boolean、实体阵列/镜像是同 Body 的固定边界，不能移动或跨越。已有 `FEATURE_DELTA` 范围的有序成员集合保持不变。草图和其他 Body 不自动随特征移动。

正式 `MOVE_FEATURE` 冻结有序 `featureIds`、逐项定义摘要和全局顺序摘要，由后端核验集合、连续性与固定边界；单项也按长度1的集合处理。从全局 `model.Features` 一次提取集合，保持内部顺序，整体插到锚点之前/之后；未选对象相对顺序不变，同步全部 `Order`，不循环提交单项移动，不建立持久特征组。每项受影响定义在同一 ChangeSet 内补偿，Feature/Parameter ID、名称、Body、参数及显式引用不变。取消和预览不写 Head，输入或 Revision 变化不能复用旧候选；提交沿原幂等、CAS 和 Undo/Redo 链。

按候选顺序重建 Body 前驱和依赖图，检查全 Part 的轮廓、孔位、支撑、终止面、外部投影及拓扑输入。语义 Anchor 和实际来源阶段分别验证；未显式保存阶段的既有引用从其不可变来源 Revision 的精确 Body Tip 核对，不补写或重绑引用。所有活动引用和 Body 的实际几何必须成功，失效指明阻塞特征并原子拒绝，不接纳 FAILED 移动。顺序与候选前驱进入原阶段输入、缓存和增量求值。

入口：`services/internal/workspace/feature_move.go`、`feature_insert.go`；真实 Router/Worker 回归 `feature_move_integration_test.go`、`feature_insert_integration_test.go`，浏览器 `solid-feature-live.spec.ts`。

## 在历史位置创建

结构树的对象菜单“在此步骤后插入”开放圆角、倒角、拔模、抽壳，以及独立 Part 或当前可编辑 READY 的直接 Product ContextVariant 中的草图、拉伸添加/切除、旋转、Sweep、孔和放样。生成器可消费该位置之前的合法草图，保留同 Body 的添加、切除或交集；双侧、对称、贯穿与到平面沿各生成器的普通合同验证，不再限定紧随草图的有限拉伸。新 Body 仍通过单独的 Body 生命周期创建，不在旧 Body 的插入命令内隐式创建。

显示和拾取统一使用冻结 Revision、occurrence 与全局插入前缀；取消、提交、Revision 或目标改变后退出，不建立持久历史游标。普通生成器面板支持编辑或创建输入草图再返回。既有生成器的新输入插在消费者之前，新历史生成器的新输入创建后，其插入边界推进到新草图；保留原 Feature/Parameter ID、原始数量文本和最新接受快照。首个实体之前复用既有空 Body 的只读可视化前缀制品：SourceFeatureID 为空，不授权捕获后续实体面或边。

`afterFeatureId` 是全局数组插入边界，`SourceFeatureID` 是该前缀中来源 Body 的实际实体阶段，不能将草图边界或语义 Anchor 当作实体阶段。`feature-input` 的独立 `afterFeatureId + bodyId + versionId` 模式重建精确前缀：圆角/倒角选边，拔模/抽壳选目标 Body 的面。GeometryKey、来源 Revision、Body、实际 SourceFeatureID 与 PersistentSelection 在创建端再次验证。目标 Body 的首个生成器前可读取空前缀，但空前缀不能授权拓扑拾取；跨 Body 参考必须有该前缀中的真实上游实体阶段。拔模实体中性面也经该接口绑定，`referenceBodyId` 单独指定同 Part 来源 Body，按同一全局历史前缀取其有效阶段；显示与下载使用相同授权。标准/自定义基准面按递归依赖顺序检查。ContextVariant 插入按冻结的 Product Revision、Variant 和 occurrence 重建已接受投影的目标/参考 Body 前缀，GeometryKey 与显示/拾取阶段一致；正式 PersistentSelection、实际来源阶段和 CreationEvidence 仍用独立定义校验，不保存投影拓扑或回写投影参数。预览、直接提交及携带预览确认共用此校验，过期上下文拒绝。该模式与编辑输入、只读 `ResultStage` 不混用，不能用最终结果、下游或 displayFallback 冒充历史输入。

原 `CREATE_SKETCH`、`CREATE_SOLID_FEATURE` 和 `CREATE_MODIFY_FEATURE` 携带临时插入信息，在内存候选中直接插入并同步全局数组/Order；按稳定 ID 读取新定义。新 Feature、参数与后续 Order 变化进入同一 ChangeSet/Transaction，沿既有预览、幂等、CAS、补偿历史与缓存提交。原定义和参数身份不变。插入和移动复用候选依赖、实际来源阶段、全 Part 引用/几何检查；下游或跨 Body 失败指出阻塞对象并原子拒绝。固定节点保持相对顺序，允许以其合法输出为锚点，不套用移动的固定边界规则；已有 FEATURE_DELTA 的有序成员集合不能改变。

历史草图仅使用合法基准面或目标 Body 的历史平面支撑；编辑显示、捕获和投影不暴露下游或失败回退。创建后按响应中的新增稳定 Sketch ID 进入原编辑器，不按列表末项推断。逐次 EDIT_SKETCH 各自提交，生成器再单独提交；每次按当前 Revision 重建输入和预览，取消生成器保留已确认草图，空白新草图沿原放弃规则清理。Product 仅开放直接、可编辑且 READY 的 ContextVariant；Part Revision、Product Revision、Variant、完整 InstancePath 与边界共同冻结。每次编辑后复用 contextProduct/baseView 刷新，重新取得已接受前缀；未就绪不回退独立默认值。活动 occurrence 独立过滤实体、草图与顶层／Part 参考几何，兄弟 occurrence 保持完整显示，不写持久显隐。草图编辑平面和支撑预览应用 occurrence 位姿。

上下文 FACE 支撑经 `contextDefinitionPick` 唯一对应到独立阶段；正式 PersistentSelection、来源 Revision／实际阶段、CreationEvidence 和右手框架都取自独立定义，显示 GeometryKey 只校验已接受投影。预览检查独立默认构型与受影响重复实例，确认沿 `commitContextPartEdit` 重放独立定义并在既有双文档事务中原子提交；接受输入保持原快照，不顺带读取来源 Head。采用预览也不回写投影模型。取消、目标切换、作用域变化和迟到回执不能恢复旧输入或预览。

入口为 `services/internal/workspace/feature_insert.go`、既有 `applyCreateFeature`、`historical-creation.ts`、`sketch-support-editor.tsx` 与 `solid-feature-editor.tsx`。定向 Router/Worker 回归为 `feature_insert_integration_test.go`，选择回归为 `feature-picking.scenario.mjs`。覆盖支撑、区域、范围保护、下游／跨 Body 原子失败、独立提交、Undo/Redo、重开及热冷 BREP/Naming。已有浏览器覆盖历史旋转及首个生成器输入往返；不开放深层／PINNED／非 READY 上下文、历史 NEW_BODY 或跨 Body 历史创建。其余组合以当次检验为准，不扩大历史移动的固定边界。

## 阶段、命名与历史

依赖连接上游 Feature 输出，而不连接 Body 的未来最终结果。局部修改先重建自身之前的 Body 前缀，再以保存的 source Revision、Body 和 PersistentSelection 解析当前输入。缺失、歧义或类型变化必须失败，不能用最近几何或持久化 local ID 恢复。显示层的 local pick 只用于创建正式持久选择。

拔模所选角度面的单一 Modified 输出以 DRAFTED_FROM 归属拔模，并沿原面保持 Modified 延续；未变面/边不改归属。真实分裂/合并仍走原规则，不猜测面对应。生成历史、Boolean、same-domain unify 和局部修改的真实 Generated/Modified/Deleted/存活关系形成同一命名链。抽壳同时保留外面并生成内面时保留两类来源；多对多来源不能生成重复 semantic ref。未被同类型历史覆盖的边/点继续使用已有语义邻接闭包，完整门禁覆盖每个 Face、Edge、Vertex。控制面 evaluator 以[求值入口](../../../services/internal/workspace/service.go)为准，Naming evaluator、策略 ID、digest 与容差以[命名策略源码](../../../services/internal/modelcore/topology_naming.go)为准；缓存包含版本和完整来源历史。

精确边的直线识别由同一个内核判定供类型查询、方向属性与 Naming evidence 使用。非周期 B-spline 在实际裁剪参数区间内，正权控制点全部落在端点直线的 `Precision::Confusion()`（1e-7 mm）容差内，且沿直线单调排列时，识别为 `LINE`；不依据显示折线或有限采样。原 BREP 和真实 History 保留，属性同时报告 `underlyingCurveType=BSPLINE_CURVE` 与原生参数。选择 evidence 使用解析直线原点、单位方向和端点距离参数，现有轴/方向解析及外部几何投影消费同一证据，不能把 0–1 的样条参数当作毫米。弯曲、回折或周期样条保守保留曲线类型。evaluator 版本进入缓存键；旧制品须经正常特征编辑/重新求值生成新的成套 Naming/Visual，再选择其边。

Feature 定义编辑保持稳定 ID、顺序和名称；生成和局部修改保持所属 Body。参数 Source 的更新与新增/移除和定义共用一次 ChangeSet。结构树可抑制/解除抑制所有建模实体 Feature，专用 `SET_FEATURE_SUPPRESSION` 命令检查 definition digest 并只改变显式 suppressed 字段，沿用 entity ChangeSet 和 Undo/Redo；导入基础实体保持受保护。失败状态与诊断为派生值，不进入可补偿的 Feature 定义。

## 用户操作与编辑上下文

所有实体 Feature 面板共用紧凑表单与视区选择：轮廓、轴、边/面、Boolean 输入、放样截面、Sweep 路径草图/曲线和薄壁线链直接拾取，不用几何下拉列表。运算、范围及连续性使用分段按钮或枚举。支持先选对象再启动或先启动再选；尺寸、单位及公式在表单编辑，不提供视区尺寸手柄。

- 面板中的输入按钮切换当前拾取角色。角色先过滤原始 hit，再按业务对象投影；拉伸／旋转的显式轮廓保留权威区域 ID，整草图及放样截面选整个草图；旋转轴和闭合起始边保留草图的单条实体，边/面不能被邻近顶点抢走。所有输入限定同一所有者文档、Revision 和 occurrence。基准面/轴输入临时显示该 occurrence 的参考几何，退出恢复原显示偏好；保存选择的高亮携带已提交编辑会话的完整 InstancePath 和 contextVariantKey。
- 进入实体 Feature 命令不改变相机 fit。旋转拾轴阶段的已选轮廓只作淡色上下文，轴线选择和 hover 分别使用更明显的颜色和线宽，包含轮廓自身的直线。
- 实体、布尔与阵列创建不带入命令前的选中几何；进入命令后按字段角色明确拾取。编辑与草图往返恢复沿用已接受的输入。圆角/倒角选边，拔模/抽壳选面；点击累加，再次点击取消，不要求 Ctrl 或“添加当前选择”。多选集合默认显示紧凑摘要，可展开条目定位、更换、移除；有序截面保留顺序操作。高亮显示真实几何。异步持久绑定顺序执行，不能因为快速连续点击丢掉输入。
- 放样截面按拾取顺序添加；“点端”拾取保留具体点引用，可上移至首端。直接选择仅含一个点的普通草图也会在预览/提交前绑定到该稳定点引用；多点草图要求明确选点，已接受轮廓不会因上游变成点草图而自动改类。默认自动对应，可逐截面重新自动、反向、拾取闭合边或设置圆相位；可选连接线来自当前成功预览的权威边界点，随预览失效清除。编辑控件显示已接受自动对应，人工修改以它为基线。
- 输入完整或参数修改后，250 ms 合并请求既有权威预览命令；新输入中止旧请求，旧 generation/Revision 的结果不能显示或提交。仅当前输入获得成功预览后启用提交；预览失败移除旧候选，修正输入自动恢复。取消、文档/Revision/occurrence 变化结束会话。预览显示结果和上游参考线，同时仍允许继续拾取原始输入。
- 编辑已有生成、放样、实体阵列和局部特征时通过只读 `POST /api/documents/{id}/feature-input` 重建自身之前的 Body 阶段并解析原选择，高亮显示原边面。该接口也为阶段上的新点击绑定持久选择；它不推进 Head。新绑定使用 `sourceVersionId + sourceFeatureId` 指明不可变来源，重算仍由真实 Naming 解析，不保存局部序号。已保存的旧 Revision 选择继续按既有最终输出来源解析。
- 编辑布尔恢复目标上游阶段和已消耗工具的明确输出阶段；工具的临时可见性不修改持久 `visible/consumed`。输入阶段制品下载按文档权限、当前 Revision、Feature 和精确对象范围授权。

创建/编辑、Boolean、Pattern、调整顺序及历史对照的异步错误走全局 operation feedback，保留 code/request/diagnostic 和重试。数量在失焦或确认时校验；下游需修复使用同一出口警告，仍按当前有效候选决定能否提交。引用状态与逐项修复留在列表中，错误不在表单重复显示。

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

已知限制：不支持任意自由曲面 G2、完整放样导向曲线、建模螺纹、曲线 G0 尖角 Sweep、任意局部特征范围重执行、部分覆盖终止、跨 Body 移动及不受约束的历史插入。复杂拓扑分裂/合并可能必须显式重选。常规建模与正式历史链已接通，不代表所有曲面/参数组合或所有 Feature×复制×修复组合完成验收；未运行组合属于验证缺口，不写成保证。

历史生成器扩展的回归入口为 `TestInsertExpandedGeneratorsAndExtentsThroughRouter`、`TestInsertInputSketchBeforeFirstGeneratorThroughRouter`、原 `TestInsertSketchGeneratorSourceAndRangeGuardsThroughRouter`；浏览器覆盖历史旋转的上游显示/拾取及既有首个生成器的新输入往返。测试存在不代表任何后续修改已复验。
