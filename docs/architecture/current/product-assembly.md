# Product、装配与发布

Product 模型与 Revision 是定义和引用的业务真相；求解器只消费冻结纯值。数学关系见[六类约束合同](assembly-constraints.md)，数值实现见[求解器算法](../../../kernel/assembly/SOLVER_ALGORITHMS.md)，执行方法见[装配验证](../../../tests/assembly-contract/README.md)。

## 结构与上下文

ProductInstance 保存明确引用 Revision、FOLLOW_HEAD/PINNED、局部 placement 与完整 InstancePath。共享 Part 的 occurrence 共享定义，不共享位姿和支持身份。一个多 CAD Body Part occurrence 是一个运动单元；嵌套 rigid Product 遵守 owning Product 边界，不展开为 flexible assembly。

工作台 EditSession 保存宿主 Product、唯一编辑目标、完整 occurrence、宿主/目标 Revision、工作 Body 和 activation generation。普通 Part 建模修改共享定义；ContextBinding/ContextVariant 是显式关联，不是另一编辑模式。[编辑上下文与标签](product-edit-context.md)与[建模和显示](model-display.md)拥有具体交互规则。PINNED 可查看，修改前须显式恢复跟随。普通读取/广播不打开、激活或重排标签。

Publication 用稳定目标身份，支持输出及转发；PersistentSelection 保存命名身份与解析快照，topology local ID 只作瞬时证据。Product Design Session 验证 root snapshot、typed path、目标与权限；跨 Workspace 设计事务原子检查 CAS，保存最终模型与 ChangeSet，Undo/Redo 使用正式补偿。FOLLOW_HEAD 变化由 digest-bound UpdatePlan 按叶到根接受 UPDATE_REFERENCES；失败保留合法快照与已提交子 Part，PINNED 不跟随。

参数 Publication 的关联更新计划另冻结 `parameterUpdates` 前沿（Part、源 Revision、依赖及待更新状态）。一次显式接受按参数依赖顺序预览并提交受影响 Part，最后让当前 Product 接受这些确切结果 Revision；相同 Part 的 occurrence 只重算定义，保留各自 ID 和位姿。参数源加入通知订阅图，计算不依赖页面是否打开；浏览器自动跟随已计算版本时跳过需要跨 Part 参数重算的计划，界面由“更新关联零件”确认。

接受前校验计划 digest、每个文档权限及 CAS，固定外部参数输入，不读取半途变化的 Head 作为结果。PINNED/ISOLATED 不传播，循环拒绝。失败不接纳失败候选，Product 保持原接受版本；已成功的子 Part 可保留独立 Revision，重新取得计划后恢复。该批次只更新参数外部引用，不顺带更新 Part 的其他 ContextReference。当前一次计划覆盖 Product 直接引用的 Part 及其递归参数依赖，嵌套 Product 仍沿用各自叶到根接受计划；未引入跨文档长事务。

## 关联更新与对象编辑

FOLLOW_HEAD 自动跟随已计算版本；跨 Part 参数重算仍通过“更新关联零件”显式接受。不存在独立“关联更新与修复”命令或面板。约束更换支撑从双击编辑进入；发布重定向使用发布编辑；参数和上下文输入在所属对象的引用编辑器中管理。约束树名称不追加 `[停用]`，停用状态由独立标记表达；Broken 约束保留编辑、刷新及删除，不提供单独 Reconnect 菜单。

内部 UpdatePlan 仍负责来源健康、确切 Revision、依赖顺序、计划摘要与 CAS。接受及普通 `UPDATE_REFERENCES` 检查来源 Part 的求值状态；FAILED 来源阻塞更新并保留原合法快照。待更新参数消费者先按参数计划求值，再检查结果。合法来源更新导致的 Broken/NotUpdated 按原合同保留，不增加全部 Verified 的门槛，不采用失败姿态。失败提示保留重试已授权的更新入口，成功不增加操作指导提示。

入口：[更新计划](../../../services/internal/workspace/product_update.go)、[工作台编排](../../../web/apps/cad/src/features/workbench/workbench.tsx)。真实回归：[Router/Worker 来源健康与历史](../../../services/internal/control/association_repair_integration_test.go)。骨架与消费对象引用的后续交互仅为[设计方案](../../../plans/skeleton-reference-workflow.md)，不代表新增骨架类型、任意深层编辑或带链接复制已交付。

## 定义与求值

公共约束经稳定支撑解析、组关系编译，再交给数值内核。Fix Together 保存组成员与捕获关系。驱动值可由参数表达式提供，Measured 为只读测量。保存的定义、参与求解的集合以及已接受的姿态分别记录。来源断开或求值未收敛时，保留诊断与最后合法构型。[约束类型与接纳条件](assembly-constraints.md) · [机构定义与位姿](kinematics-dmu.md)

## 编辑候选

预览和提交使用相同的支撑、顺序、方向、数值与表达式。候选绑定 actor、文档、Revision 和输入摘要。只有匹配的预览才能提升为正式结果，修复、改参及并发 Head 变化需要重新求值。

## 连续操纵

Begin 冻结构型、支撑、约束和分支。Update 使用临时目标，在已接受的硬约束下计算姿态。前端保留一个在途请求和最新待发送目标，释放时以最终目标完成资格检查。有实际变化的手势只产生一次 Move Revision。取消、超时或过期不会采用未确认姿态。硬可行性、运动偏好收敛和最终提交资格分别判断。[连续操纵合同](assembly-constraints.md#尊重约束的连续三维操纵)

## 局部诊断与剩余运动

诊断冻结当前 Product 的约束邻域和构型，按刚性单元展示平移与转动方向。自由度、整体参考框架和冲突证据分别计算。缺少当前 Revision 的证据时可只读刷新，未决结果保持未知。[数值求解与诊断](../../../kernel/assembly/SOLVER_ALGORITHMS.md)

## 装配依赖分析

`assembly.dependencies` 是独立的只读 UI Command，通过声明式目录进入装配工具栏和命令搜索。选择组件可将其设为分析根；默认显示约束，支持逐层／全部展开、收起、重新设根、分类过滤，以及图内缩放、平移和临时移动节点。图中选择映射到既有结构树／视区选择；约束的编辑与启停复用原有编辑器和带版本的正式命令，Viewer 不能执行写入。隐藏三维图标与停用关系分开显示。

模型投影采用当前编辑 Product 的直接刚性实例、约束（含角度参考轴及固联组成员）、上下文绑定，以及当前 Product 参数的已检查 `reads` 和外部参数发布引用。组件与约束为二部关系，约束连线不表示因果或求解顺序；上下文及公式连线保留来源方向。不按参数文本、显示名、位置或共享 Part 文档猜测关联实例。断链对象保持可见诊断节点；缺少已检查引用时明确标记不可用。孤立断链约束仍从装配根可达。已访问集合终止循环网络和重复展开，图节点身份由 owning document、occurrence 和稳定实体 ID 组成，Revision 用于操作资格验证。

[GraphView](../../../web/apps/cad/src/components/graph-view.tsx) 隔离 React Flow 的渲染／手势状态；[装配投影](../../../web/apps/cad/src/cad/assembly/assembly-dependencies.ts) 不依赖图组件。React Flow 使用 MIT 许可，按需加载；展开、节点位置及缩放不形成 Revision 或 Undo/Redo。显示采用紧凑图标与名称的依赖展开树：装配根先列关系，再展开实际支撑；组件根先列自身关系。父节点按可见子树的上下边界纵向居中，从左侧中部向右展开，列距考虑标签宽度。重复业务对象可有多个临时显示 occurrence，选择与操作始终映射回同一个稳定实体；每个实体仅展开一次，其余作为终止引用，循环网络的显示规模由节点与边数约束。公式／上下文箭头仍表示真实来源方向，向左连线不反转语义；普通约束连线没有箭头。默认仅展示紧凑状态标记，完整状态见悬停与选中详情。选择保留已测尺寸与当前查看范围；仅在展开拓扑变化或整理布局时自动适合窗口，新增节点先测量再启用视区裁剪。节点位置不持久化，可重新整理布局。子装配内部关系需进入所属 Product；本页不递归抓取未接受的 Part Head，也不声称覆盖全部跨文档公式、所有建模上下文依赖或具有工业规模的图布局性能。

CATIA 参考：`online/asmug_C2/asmugbt0608.htm`（Analyzing Degrees of Freedom）描述当前激活装配范围内的平移／转动方向、坐标与逐方向高亮；`online/asmug_C2/asmugbt0607.htm`（Analyzing Dependencies）描述组件—约束—关联组件的展开树、重新设根、激活切换、缩放，以及 Constraints／Associativity／Relations 分类。前者的黄色方向反馈与后者的操作组织用于交互参考，不据此推断 CATIA 的数值或图布局内部实现。本项目保留自身的证据门、刚性单元范围和正式引用身份；图组件依据 [React Flow MIT 许可与组件职责](https://reactflow.dev/) 选型，不使用 Pro 示例或额外布局引擎。


## 静态干涉与最小间隙

Product 的“空间分析 → 碰撞”计算独立只读，复用 `AnalyzeInterference`。范围支持全部零件、所选内部、所选与其余、两组之间；树／视图区使用同一选择会话，子装配展开为接受的叶零件，两组重叠时去重并排除自身。隐藏不排除；已消费 Body 排除，无活动实体的零件计入范围说明。同一叶 Part 内 Body 不互检，不同叶零件（包括同一模块内部）按 Body 配对。

`static_interference.go` 从确切 Product Revision 的接受引用展开完整 InstancePath、Part Revision、ContextVariant、Body、精确 BREP 和组合位姿，不从显示行建立范围，不读取来源新 Head。失败 Variant、失败实体、缺失 BREP 与 displayFallback 不能充当有效输入；未完成配对保留 INCONCLUSIVE。零对、取消、超限或部分失败不报告全部通过；已有冲突与未完成分别表达。开始前预检 128 Body、8192 对、256 MiB 去重 BREP，不截断。

OCCT 在私有几何上计算分离、接触、干涉及包含；包含通过真实残余差集校验，不以总体积相近代替。Worker 使用有界 BREP／已准备形状及精确配对缓存。阈值检查可用保守包围距离证明间隙满足，此时不生成虚假的精确距离或见证点；选中配对后按需求精确距离、真实 Section 交线及 Common 交叠体的临时网格。图形离散不参与工程分类；局部显示超限／失败不改变已完成的数值判定。包含／干涉不绘制虚构的穿透向量，当前没有穿透深度算法。接触提供可靠见证，未实现完整接触／间隙区域面。

面板提供问题／类型过滤、按配对或零件组合浏览、工程数值和覆盖计数。行选择定位两个实体，用细线轮廓区分角色，并以三角线网格叠加交叠材料，保留原零件材质；使用紧凑对话框，可临时只看这一对；剖切由独立“切割”命令提供。保存碰撞对象的树选择也按同一几何／位姿补取当前配对的精确交叠网格，不借用旧位姿结果。临时显隐在关闭、换范围、取消或版本变化时恢复，分析不写引用、位姿、Revision 或历史。图形必须匹配冻结的 Revision、几何与实际显示位姿；源、策略及内部诊断只进入 DEBUG。

可选择已保存机构的接受构型进行同样的只读检查，无需激活、求解或采用其位置；视口与报告位置不同则不显示标记。命令试动／录制内的采样检测及停止边界见[机构会话](kinematics-dmu.md#试动和录制会话)。

入口：`POST /api/documents/{documentID}/static-interference`（Viewer 可读）、`services/internal/geometry/dmu.go`、`kernel/occt/src/dmu.cpp`、`static-interference-panel.tsx`；真实回归在 `static_interference_integration_test.go`，状态／图形在 `static-interference.scenario.mjs`，浏览器在 `assembly-engineering-live.spec.ts`。不提供连续碰撞保证、自动修复、穿透深度或报告导出；最大资源规模和复杂导入实体仍需专项验证。保存与手动更新见下节。

碰撞面板采用范围／间隙输入、单条检查状态、零件对结果列表。每行保留两件名称、分类及适用的工程量：交叠显示精确交叠体积，分离且距离已精确计算时显示间隙；接触和未完成不填充无意义的零值。选择一行计算其局部精确图形，可临时隔离所选零件对；按零件分组与结果过滤保留。检查可取消，保存只接受与当前定义、Revision 和构型一致的完成报告。“随文档保留”默认启用，确定保存成功后退出，取消退出；机构临时姿态只检查而不保存为空间分析对象。

此交互参考 CATIA **Reading Clash Command Results**（`online/spaug_C2/spaugbt0303.htm`）的一冲突一行、选中后详细计算与 OK 保留结果；本实现的两列呈现和确定／取消为产品设计选择，不代表完整 CATIA 报告、矩阵或冲突处置流程。

## 空间分析对象与手动更新

切割中的平面是无限几何，宽高只属于视区显示，不提供平面宽高数值输入；薄片保留厚度，有限切割框保留三向尺寸。截面视图通过开关控制独立查看窗口；窗口生命周期不依赖求交结果，未相交或等待结果时保持空白，外框按页面限制为不超过 720px／72vw 宽、560px／65vh 高，几何变化不改变窗口大小。支持滚轮缩放、中键平移与适合窗口。每次收到新的非空截面采样自动适合并复位手工缩放／平移，静止时保留用户调整；空截面沿用最后查看范围而保持空白。中键手势支持指针捕获，Esc／失焦／指针取消还原本次平移；不另建分析或 Revision。切割使用“确定／取消”：确定经正式保存成功后退出，取消只结束临时会话；没有独立分析或保存按钮，过期定义打开后自动计算。只读用户确定仅退出。完成状态不重复显示“分析完成”。

当前 owning Product 的“应用 → 空间分析”保存碰撞、切割、距离／距离区域对象；不混入机构定义或旧 Motion 静态分析分支。计算和查看只需读权限，保存／编辑／删除／更新经正式命令、Preview/Commit、CAS、ChangeSet 与 Undo/Redo。保存定义包含稳定 occurrence 路径、参数、局部切割框架和已接受结果引用；JSON 图形／报告存入独立不可变 `SPATIAL_ANALYSIS` 制品，读取校验所属文档、引用、SHA 与来源快照。取消不留下树节点。

全部零件或所选范围沿既有接受装配解析，包含嵌套叶 Part、各接受 Revision、ContextVariant、Body 和组合位姿；不采用来源新 Head 或临时操纵位置。距离复用精确实体配对最小距离。切割支持无限平面、双平面薄片和切割框；内核在私有精确 BREP 上求交，保留孔和独立截面，返回精确面积／重心及用于显示的离散边界／三角形。薄片面积为两截面之和，框为六面截面的总面积；表示宽高不裁断无限平面／薄片。三维切除仅为视口裁剪，不创建或修改实体，关闭后恢复。

切割打开即显示临时参考平面与现有局部操纵器，新建时按范围包围盒居中并自动计算一次；已有有效结果直接读取，不重复计算。拖动主平面沿法向移动，Shift 或先左键后中键沿局部 UV 平移；轴箭头平移、轴旋转手柄旋转，边缘／厚度手柄缩放并固定对侧边界，位置／旋转／尺寸字段同步。鼠标中键先按仍用于原有视图导航，Esc 先回滚当前手势。拖动立即更新临时裁剪与 stencil 封口，并每 80ms 合并最新输入到单路精确计算队列；同时最多一个请求在途、一个最新快照等待，中间快照被合并。拖动期间可显示最近完成的截面采样，松开后只有匹配最终定义／Revision 的结果可保存；计算速度受精确 BREP 求交耗时限制，不保证每个鼠标事件都产生精确结果。关闭、取消、版本改变清理临时材质、选择和迟到响应，不修改正式显隐或模型位姿。

“几何定位”复用当前接受版本的支持解析器，以完整 occurrence 路径校验顶点、参考点、坐标系原点、平面、直线边、轴、圆边或圆柱，并将精确支撑变换到装配坐标一次；点定位保持切面的当前旋转，仅移动到所选精确点。这是一次定位，不建立新的目标关联。定位拾取期间暂停临时裁剪，以免隐藏可选支撑。截面视图沿当前局部框架正视显示既有计算的边界与填充，支持适合／缩放、定位零件以及切换薄片／框的单个截面；显示离散不参与面积计算。尚不支持任意曲面／曲线切向跟随、二维测量编辑或直接拖动截面曲线。

切面助手与实体共用深度缓冲：可见面低透明度填充，被遮挡边界用淡虚线提示。三维切除统一为“仅截面／内侧／外侧”；后两者用 stencil 对每个 Body／切面计数生成实时封口，保留孔洞，斜线 shader 以 CSS 像素维持线距并参与深度遮挡。显示封口不取代精确 BREP 面积与持久结果；每 Body／切面增加两次计数与一次封口绘制，复杂装配性能尚需专项测量。显示精度用对数滑条调节 10–0.01mm，向右更细；仅影响显示离散，界面提示按当前长度单位换算。

距离区域对不重叠的两组零件计算第一组表面到第二组实体集合的距离。OCCT 曲面参数采样点采用精确点到实体距离，包围盒仅剪枝；按显示精度有界细分三角网格，红色表示小于最小值、绿色表示区间内，灰色保留无法确定的边界。区域网格及边界是近似显示，不是精确几何分类或面积测量；没有证据时不染为合格。单次最多 16 个第一组实体、每实体一万个距离查询，深度／图形／时间超限明确未完成，不保存为已完成结果。总冻结资源仍受既有 128 Body／8192 配对／256 MiB BREP 门禁，图形另有总量限制。

性能与复用：空间分析同一请求共享接受装配读取缓存。Worker 内核按精确来源几何、组合位姿、配对容差缓存完整局部图形，截面／距离区域另外包含目标集合、切割框架、尺寸、范围与显示精度；两类图形载荷各有 8 MiB／128 项预算，只缓存完成结果。改间隙阈值仍重新判断通过状态，修改构型／范围／精度不复用旧图形，取消优先于缓存返回。截面只在保守 BREP 包围盒证明无交时跳过精确布尔，保留接触容差余量。

前端按 16 MiB 载荷预算仅缓存源 BufferGeometry 的轮廓 CPU 数据，位置／索引属性或版本变化失效；每次叠加仍独立拥有并释放 GPU 几何。剖切材质更新按 Body 索引线性处理，不扫描全部实体查找每个材质。复用不修改正式材质、显隐、模型或历史。

机构实时干涉与静态检查使用同一交线／交叠网格 renderer。运动延续探针只做数值检查，最终显示的采样构型才按同一冻结 BREP 与位姿补取接触／干涉图形；局部图形失败明确未完成，可检查当前位置重试，不改变数值分类或停止判定。被拒绝的碰撞构型不在保留的安全姿态上绘制。 STOP 的最后一个数值探针只有冻结摘要、确切位姿与序号均一致时才复用为最终检查点；多组干涉逐对发送两个精确实体，遵守 Worker 单配对局部图形合同。

性能验证入口：`spatial_performance_integration_test.go` 记录真实 Router/Worker 的串行小场景服务耗时、包含在其中的分析 RPC 耗时与次数；`web/apps/cad/scripts/benchmark-spatial.mjs` 仅测合成显示网格的 CPU 轮廓刷新／剖切材质更新，不含 GPU、网络或精确几何计算。持续变化构型的精确交集仍是主要成本；重复输入加速不代表大装配容量、稳定 P95 或实时帧率。

首轮统一手动更新：来源摘要包含有效定义、完整稳定路径、接受叶版本、位姿与相关 Product 输入赋值；排除分析自身保存产生的 Revision 和显示名称。现有依赖图登记分析对实例／关联输入的读取，当前投影将摘要不同标为“待更新”、缺失来源标为“来源不可用”。它不触发计算、求解、自动跟随或新回调系统。节点“更新分析”或分组“更新全部分析”显式重建；全部候选成功才原子替换结果，失败保留上次有效结果。旧结果可读但只有当前几何／位姿匹配才显示标记，取消、关闭、范围或版本变化使迟到响应失效。结果制品丢失可显式更新重建，不借用其他版本的结果。该定义／依赖／接受结果分离方式可供后续领域复用，尚未实现全局自动更新、冻结策略或通用更新调度器。

入口：`spatial_analysis.go`、`spatial-analysis-panel.tsx`、`geometry/spatial.go`、Worker `AnalyzeSpatialGeometry` 与 OCCT `dmu.cpp`。回归：`spatial_analysis_integration_test.go`、原静态干涉测试、`static-interference.scenario.mjs`、`section-manipulator.scenario.mjs`、`assembly-engineering-live.spec.ts`。CATIA 对照：`spaug_C2/spaugbt0303.htm`、`spaugbt0306.htm`，`ccvug_C2/ccvugbt0700.htm`、`cfyugspa_C2/cfyugspabt0201.htm`、`cfyugspabt0203.htm`（直接操纵）、`cfyugspabt0204.htm`（目标定位）、`cfyugspabt0205.htm`（数值位置）、`cfyugspabt0206.htm`（截面视图）、`cfyugspabt0212.htm`、`cfyugspabt0103.htm`、`cfyugspabt0102.htm`；参考中的自动／冻结更新、完整二维测量编辑、连续碰撞与报告管理不代表当前能力。

## 发布与关联设计

Publication 以稳定接口 ID 暴露参数或几何，也允许经 Product 转发。别名用于界面，消费者绑定所属定义、接受版本、接口及 occurrence 路径。更新时重新解析来源，缺失或不兼容的接口需显式替换。树选择发布条目时可定位来源几何。

当前 owning Product 的直接 Part 实例可通过 ContextBinding 将发布参数、平面、轴、点或支持的曲线绑定到本地槽位。参数保留 ParameterId，基准保留 DatumId，草图外部输入保留稳定槽位。ContextVariant 保存实例上下文的结果，同一 Part 的两个实例可以有不同输入。共享 Part 定义的修改会影响其全部引用。任意曲面与 Body 的跨层几何输入尚未开放。

更新计划冻结来源和接收方版本，检查权限、单位、输出类型、循环及各 Head 的 CAS。固定来源保留明确版本，解除或更换来源由消费对象自己的命令完成。[编辑上下文](product-edit-context.md) · [骨架与引用设计](../../../plans/skeleton-reference-workflow.md)

## 显示、历史与发布

新 DocumentView 即使 GeometryKey/pose 不变，也同步 owner/version/referenceMode、InstancePath、选择与操纵绑定；开始手势从当前权威快照按稳定 occurrence 冻结，旧路径严格拒绝。约束显示按 owner/occurrence/ConstraintId 增量 reconcile，增删、模式、抑制、评价、支持与标签独立失效，请求帧并清理拾取/高亮，无 GLB 下载或完整 BVH 重建。单一手柄吸附复用可见拾取与精确查询；中心重定位不是模型移动，手势框架冻结。

SolveManifest 冻结用户定义、编译输入、descriptor/来源、occurrence、参数解析值、模式/激活、组阶段、分支/基准及 policy/profile/build；失败试算和采用结果分别记录在操作诊断与 SolveManifestResult。Replay 不查询最新几何、不依赖 Session。开发数据只支持当前 DefinitionVersion=2、Quantity 和 assembly-m4m5-reference-retraction-v13/canonical JSON 摘要；旧实验定义、Offset 符号约定、pair-Rigid 持久记录和 policy 明确拒绝，不做读取修补、换号、组迁移或多代投影。Rigid 快捷创建仍生成当前正式组，内核刚性原语继续使用。已有旧开发数据可能无法读取，未自动重置。当前格式的 JSONB 数值规范化、篡改拒绝、Revision、CAS、Undo/Redo 与求解快照重放完整保留；记录实际重放 build，不承诺跨版本逐位一致。

## 入口与限制

- services/internal/workspace：product_context、product_design_transaction、product_update、assembly_solve、assembly_interaction、assembly_interaction_continuation、assembly_conflict、assembly_engineering、assembly_solve_manifest。
- kernel/assembly：solver.hpp、solver.cpp 与算法文档。
- Web：assembly-interaction、motion-presentation、assembly-conflict-panel、cad-viewport-engine。
- 定向测试、报告和人工步骤见装配验证，不把测试存在或历史通过当作当前执行。

尚未解决的问题见下表；开发优先级统一由 [CAD 核心主线](../../../plans/cad-core-roadmap.md)维护。

| 当前限制 | 证据与处理边界 |
|---|---|
| 硬可行但运动偏好未收敛 | 已保留真实失败输入，可用生产 Worker 离线重放；增加预算或改善初始猜测有成功记录，未交付通用收敛改进或有界恢复算法。保持硬约束、运动优先级、容差、分支和候选资格；不能将失败姿态提交，或把偏好未收敛称为无解。重建入口为下节 CLI 和 `tests/test.data/assembly-notupdated-product.3dreplay`。 |
| 剩余运动待计算 | 缺少当前 Revision 证据时可独立只读刷新；冻结构型不满足、数值失败或证据不完整的范围仍保持未知，不借用旧 Revision 或关系数量。 |
| 交互与显示细节 | 维护者反馈仍有问题，但尚缺具体操作、对象和可复现输入；不推断根因，也不称为已解决。复杂闭环、视觉和故障恢复须按具体场景补浏览器证据。 |
| 数值与规模 | dense 后端、局部分支、单次分解不可中断和非线性 UNKNOWN 仍有效。端到端/60 Hz、并发和工业容量未完整验收；现有小样或千行 UI fixture 不证明求解容量。 |
| 未交付范围 | Engineering Connections、flexible assembly、通用最小冲突证明、稀疏后端和跨主机协作。 |

### 装配失败诊断与最小重放

装配试算失败经现有 operation diagnostic repository 记录不可变 Product 基线/候选、来源 Revision、排除约束集合及精确数字重放。admission 隔离为 NotUpdated 的定义仍携带 `evaluationFailure.diagnosticId`，普通预览显示工程问题与恢复动作，原始失败信息集中到DEBUG开发信息面板；失败位姿不能进入已接纳集合。整体/相对 DOF 对应已接纳求解集合，不能用它替代被隔离试算的收敛证据。

DEBUG页签统一提供独立开发信息面板，属性与历史保留工程信息。现有后端诊断入口继续保留。装配下载 `GET /api/documents/{id}/assembly-diagnostic` 只读冻结当前 Head，输出 `occccad.assembly-diagnostic.v2`：全部原始定义/评价/停用/测量状态与来源表、精确 body-local 几何/数值约束表、当前姿态/初值/profile/参与集合、组阶段和递归来源 Revision/Part Body 身份。默认不附带完整 Part 模型历史；`?naming=true` 按需附加去重 Naming 证据。解析失败逐约束记录端点/阶段，其他可解析数据继续导出，不造假几何。导出不求解、不写 Revision/历史/暖启动。

NotUpdated 通过 `evaluationFailure.diagnosticId` 关联归档的真实失败输入：实际 RPC 顺序、组阶段初始姿态/initialGuess、参与集合、有效配置、分支/意图、预算、结果/错误及实现 fingerprint。缺失或裁剪的历史显式标记，当前状态不能冒充原现场。数值捕获/导出限 8 MiB，操作序列限 4 MiB/32 帧，异步队列限 32 MiB；可选轨迹/矩阵采集有独立上限，落盘失败不破坏正式操作。

完整 `.3dreplay` 仅有 snapshot 表引用这一数值来源；普通求解与回放共用 Go 编译器，按端点所属 occurrence/身份生成短 `g000000` ID，C++ 在 nominal 分支冻结后绑定几何/刚体/cluster 索引，残差和 Jacobian 循环不用长字符串拼接查找。持久身份、提交 digest 和结果的稳定 constraint ID 仍保留。默认输出计数、逐约束残差、秩/退化、候选和阶段耗时，完整矩阵/轨迹按需开启。

离线入口使用同一生产适配器/C++ Worker，无数据库、S3、BREP 或在线解析。支持当前已接受集合、显式加入 NotUpdated、全部未停用驱动约束、保留固定/第三轴/组边界的完整关联子系统，以及真实失败当次的顺序回放。调试初值/分支/预算记录在派生结果，不覆盖原文件或文档。可行、偏好未收敛、输入错误、数值未收敛、执行失败分别呈现；只有已编译分支及固定姿态下、无可动参数的明确矛盾证据分类为已证矛盾，不将超时、秩亏或局部失败认定全局无解。工具、字段和限制见[CLI](../../../services/cmd/occccad-3dreplay/README.md)。

产品版本（ProductRelease）当前已移除：无创建、列表、重放及发布版本导出入口或存储表；后续重新设计。普通 Revision、命名版本、文档历史、接受引用和独立 SolveManifest 重放保留。
