# Product、装配与发布

Product 模型与 Revision 是定义和引用的业务真相；求解器只消费冻结纯值。数学关系见[六类约束合同](assembly-constraints.md)，数值实现见[求解器算法](../../../kernel/assembly/SOLVER_ALGORITHMS.md)，执行方法见[装配验证](../../../tests/assembly-contract/README.md)。

## 结构与上下文

ProductInstance 保存明确引用 Revision、FOLLOW_HEAD/PINNED、局部 placement 与完整 InstancePath。共享 Part 的 occurrence 共享定义，不共享位姿和支持身份。一个多 CAD Body Part occurrence 是一个运动单元；嵌套 rigid Product 遵守 owning Product 边界，不展开为 flexible assembly。

工作台 EditSession 保存宿主 Product、唯一编辑目标、完整 occurrence、宿主/目标 Revision、工作 Body 和 activation generation。普通 Part 建模修改共享定义；ContextBinding/ContextVariant 是显式关联，不是另一编辑模式。[编辑上下文与标签](product-edit-context.md)与[建模和显示](model-display.md)拥有具体交互规则。PINNED 可查看，修改前须显式恢复跟随。普通读取/广播不打开、激活或重排标签。

Publication 用稳定目标身份，支持输出及转发；PersistentSelection 保存命名身份与解析快照，topology local ID 只作瞬时证据。Product Design Session 验证 root snapshot、typed path、目标与权限；跨 Workspace 设计事务原子检查 CAS，保存最终模型与 ChangeSet，Undo/Redo 使用正式补偿。FOLLOW_HEAD 变化由 digest-bound UpdatePlan 按叶到根接受 UPDATE_REFERENCES；失败保留合法快照与已提交子 Part，PINNED 不跟随。

参数 Publication 的关联更新计划另冻结 `parameterUpdates` 前沿（Part、源 Revision、依赖及待更新状态）。一次显式接受按参数依赖顺序预览并提交受影响 Part，最后让当前 Product 接受这些确切结果 Revision；相同 Part 的 occurrence 只重算定义，保留各自 ID 和位姿。参数源加入通知订阅图，计算不依赖页面是否打开；浏览器自动跟随已计算版本时跳过需要跨 Part 参数重算的计划，界面由“更新关联零件”确认。

接受前校验计划 digest、每个文档权限及 CAS，固定外部参数输入，不读取半途变化的 Head 作为结果。PINNED/ISOLATED 不传播，循环拒绝。失败不接纳失败候选，Product 保持原接受版本；已成功的子 Part 可保留独立 Revision，重新取得计划后恢复。该批次只更新参数外部引用，不顺带更新 Part 的其他 ContextReference。当前一次计划覆盖 Product 直接引用的 Part 及其递归参数依赖，嵌套 Product 仍沿用各自叶到根接受计划；未引入跨文档长事务。

## 关联更新与修复

工作台“关联更新与修复”聚焦当前 owning Product 直接引用的 Part。按零件分组、按稳定 occurrence 路径分别显示重复实例的名称、FOLLOW_HEAD/PINNED、已接受及候选 Revision、来源阻塞与实际诊断；PINNED 明确为固定版本。定位实例、约束或支持使用当前 Revision 与原 occurrence，不按同名零件或几何猜测合并。

约束及支持状态来自当前已接受 Product Revision；缺少支持解析快照时显示“待检查”。UpdatePlan 的 `ASSEMBLY_SOLVE` 也是已接受状态，`affectedConstraintIds` 只是保守集合，不是候选版本的逐约束结论。本入口不做更新前影响预测；Revision 变化后旧计划操作失效，刷新前不允许接受或重算。

普通 FOLLOW_HEAD 仍自动跟随，参数更新仍需明确“更新关联零件”。面板复用原计划接受、重试、Refresh、Edit/Reconnect 与权威预览/确认，重算按钮不会隐式接受待更新参数。接受及普通 `UPDATE_REFERENCES` 检查确切来源 Part 的求值状态；FAILED 来源阻塞并指出 Part、Revision 和失败 Feature，保留原合法快照。待更新参数消费者先按原参数计划求值再检查结果。合法零件更新导致的 Broken/NotUpdated 仍按原合同保留，不增加全部 Verified 的接受门槛，不采用失败姿态。

入口：[面板与版本范围](../../../web/apps/cad/src/features/workbench/association-repair-panel.tsx)、[原更新计划](../../../services/internal/workspace/product_update_release.go)。回归：[真实 Router/Worker](../../../services/internal/control/association_repair_integration_test.go)、[实际视图区重选](../../../web/apps/cad/browser/association-repair-live.spec.ts)。本入口不编排嵌套更新，不刷新 DOF，不扩展柔性装配或求解算法。

## 定义与求值

独立机构定义、硬驱动、冻结回放与产品约束转换见[当前机构合同](kinematics-dmu.md)。它复用本域冻结输入及数值内核，运行保留完整机构方程，不继承 Product 关系；运动研究不改变普通装配编辑的偏好、Preview/Commit 或 Revision 语义。实时试动与关键帧录制已有实现；具体范围及验证边界由该主题拥有，独立静态干涉应用未交付。

公共六族为 Coincidence、Contact、Offset、Angle、Fix、Fix Together；Concentric/Distance/Parallel/Perpendicular/Rigid 是内部原语或明确快捷入口。唯一生产目录在 [assemblycontract](../../../services/internal/assemblycontract/README.md)，测试目录仅引用 capabilityId 并映射执行证据；生产按一次加载的只读索引查询，返回脱离缓存的副本。按钮可用性不依赖 PASS 计数，FACE/EDGE/VERTEX 不等于精确类型。

Point/Axis/Plane/Cylinder/Circle/Sphere/Cone/Frame 及稳定派生角色经 B-Rep、Datum、Publication、持久选择解析，局部到 owning Product 只变换一次。Worker 长度用毫米、角度弧度；Quantity 源值 SI 在领域编译边界转换。mesh 不是权威支持。

Literal/表达式保存 Quantity、稳定 AST 参数引用与维度。参数、方向、支持、模式原子编辑；Measured 保留驱动定义，独立 measuredValue，不加方程或移动组件，不可测时清除旧值。mode、suppressed、连接与评价状态正交，恢复保留模式并重新解析支持。

Fix Together 保存 GroupId、成员及捕获关系；先解有效内部约束，再从成功结果冻结相对关系用于组外。嵌套/重叠确定性编排，循环拒绝；失败不覆盖捕获基准。组停用不删除内部独立约束。

已接纳集合与未接纳定义分离。支持失败可保留 Broken，可解析但不可行/未收敛按命令合同保存 Impossible/NotUpdated；失败位姿不提交，不伪造 Verified、不偷偷 suppressed。Release 检查全部需要满足的活动定义，而不只检查最终子集。

## 编辑候选

Preview/Commit 共用草稿规范化，包含支持顺序、方向、参数/表达式、模式、组成员和完整上下文。求值结果/branch 属于候选证据，传输关联不作为用户意图。输入变化、迟到响应、目标切换或取消使旧候选失效。

提交保留 actor/权限、目标、Revision/CAS、定义/意图摘要、支持证据、policy/build、TTL 与交互身份校验。预览返回候选到期时间，确认前临近到期则以同一完整草稿重新预览；到期/缓存淘汰与提交竞争时最多重取一次。传输序号由 RealtimeClient 单调分配，草稿 epoch 继续隔离迟到响应。真实 Revision/意图 stale 保留草稿并要求重新预览；有效候选提升不重新完整求解；成功重试先走 receipt。确认成功关闭编辑器，失败保留草稿。

瞬态候选意图摘要将 JSON 数字的负零规范为零，兼容浏览器对 Quantity 原始数值的 JSON 往返；其余非零十进制仍精确比较，不引入浮点舍入、容差或下溢等价。字符串、类型、字段缺失和显式 null 仍不同；持久模型与历史摘要不受此规范化影响。

合法定义完成支持解析并进入 SOLVING 后，admission 隔离、子请求 deadline、Worker 不可用或数值未完成可产生仅定义候选（DEFINITION_ONLY）。用户显式确认才保存新定义及 NotUpdated 和结构化失败；不采用失败姿态、branch、组捕获或测量。此候选不能用于 Move，也不伪造收敛 Manifest。求解子预算为父请求保留结束时间；父取消、权限/参数错误、CAS 与数据库失败仍拒绝。新增/编辑先使旧评价失效，正常重算才能恢复 Verified。

## 连续操纵

数值 Session 冻结 actor、Workspace、owning Product/occurrence、Head/sequence、引用/定义摘要、实际已接纳活动集合、descriptor、组阶段、nominal、branch、policy/profile/build。容量 128，空闲 TTL 两分钟；不占等待用户的数据库事务。重启可使 Session 失效，不损坏持久模型。

Session 拥有 Begin 上下文的值快照，复用只读 nominal/geometry/definition/compiled constraint ID 索引；accepted warm/branch 更新不改变结构。任何 owning Head/sequence、root Revision、typed path、PINNED/liveness 校验失败均拒绝使用旧 Session。EditContext 校验每次只展开一次 root snapshot，同一次校验按 document/Revision 复用只读行；RPC 前后分别使用新读取缓存，不能将 mutable Head、删除状态或授权缓存跨帧。Worker 的分解复用仅在单次纯数值 solve 内，不拥有业务真相。

在途标记授予一个 Update 对 model/warm/branch 的唯一写入权。全局 Session 锁只保护成员资格、目标序列与候选 token；求解、模型值切片复制、JSON/摘要和最终证据持久化均在锁外。取消可立即撤销 Session 并取消数值/数据库上下文；最终持久化后重新校验 base，只有仍有效的 Session 能取得候选 token。Commit 的 Session 身份检查位于上下文 I/O 的前后，最终 Revision/CAS 与幂等规则继续由正式命令保证。

连续操纵通过 Begin/Update/Cancel AssemblyInteraction Session；普通精确位姿 `MOVE_INSTANCE` 仍检查硬目标，冲突时拒绝，不能用它的 Preview 验证受限操纵。DragTarget 是交互偏好而非 interaction-driver Fix。优先级为硬约束 → driven → held 姿态/侧向 → 剩余 nominal；静态解选择独立。pivot/frame/baseline 冻结，desired 来自累计用户输入，accepted 只作 initial guess。无 Ground 的整体运动保留。

一个在途加一个最新待发送目标，合并而不取消饥饿，pointerup flush 最终目标。可行显示帧、合格 checkpoint、warm start、最终候选分开。Budget 不等于网络超时；最多四个 continuation bridge 共用 deadline 和总迭代预算，未合格子步不运输分支或生成 Revision。

Final 同目标有界续算，未确认保留明确未保存预览，可重试/取消，不能提交旧目标。工具/命令切换先终结未提交状态；取消恢复当前权威快照，不能用旧 baseline 覆盖外部更新。SPACE 捕获不改变，显式 Move 授权的 RELATIVE 基准更新仅在最终原子命令中发生。

受限目标的 Gauss–Newton 搜索停滞时，内核在原硬约束零空间使用 driven 目标的 Lagrangian 曲率寻找第二下降方向；硬精度、优先级、预算和候选资格不变。Begin/Update 异常及未合格 Final 使用现有有界操作诊断仓库，保留原始交互输入、冻结基准和实际数值请求序列；中间 Budget 帧不逐帧归档。未确认 Final 在 `evaluationFailure` 中返回实际诊断 ID、阶段和错误码，WebSocket 错误复用领域错误映射，不将其改写为通用 `ASSEMBLY_OPERATION_FAILED`。

pointerup 后冻结最终目标、Session、文档与 generation，最终求解/提交独立于普通 UI 选择。空白点击或选择其他对象不取消、不重定向、不在完成后抢回选择。模型命令通过结束屏障等待；未提交预览或回执未知须先重试/取消，不能从临时显示位置开始下一操作。显式取消与外部失效仍有效。

成功有变化的手势一次 Move Revision，全部相关姿态/证据原子保存；中间帧、取消、无变化零提交。回执未知按 requestId 查询/幂等重试，不将本地回滚解释为业务撤销。

## 局部诊断与剩余运动

只读诊断冻结目标隔离定义及相关完整邻域，不调用会缩减集合的 admission。独立满足见证为 SAT，有限解析证书为 UNSAT，一般数值失败/预算/证书不足为 UNKNOWN。不可约须原集合 UNSAT 与必要删除 SAT 证据，不是最小基数。修复经用户选择正式 Edit/Reconnect/Suppress/Measured；版本变化使旧结论失效，正常受限拖动不触发搜索。

剩余运动列出当前 owning Product 全部既有刚性运动单元。默认读取精确 Revision 的命令结果，亦可通过“刷新剩余运动”独立只读计算、取消或重试，Viewer 不需执行装配重算。刷新冻结当前 Head 的已接受引用版本及位姿，只编译未停用、非测量且 Verified 的硬约束；Broken/NotUpdated 等隔离定义列为排除项，不重新 admission。默认 `FreezeAssemblyInput` 仍为完整诊断保留可解析的隔离定义，不能把它直接当作已接纳集合。

刷新复用原编译器、组关系和数值内核，不增加临时 Fix。只有求解返回位姿与每项自由度的 linearization pose 均匹配冻结位姿、范围覆盖完整的组件才可显示方向；发生实质位移、求解失败、缺证据或覆盖不完整时保持未知，不采用失败姿态或更新 branch/组捕获。结果带 Revision、输入摘要、已接纳/排除范围、确切引用及 solver 策略版本；独立于原命令结果，以服务进程内有界缓存保存可重建证据，服务重启或淘汰后可再次刷新。分析不写模型、引用、位姿、生产 warm start 或历史。版本变化、取消、关闭及迟到响应清理标记并阻止旧结果恢复。本入口不编排嵌套更新、不扩展柔性运动单元或数值算法。

静态证据相对数值锚点，gauge 独立；交互证据是含整体运动的物理子空间。数值锚点不是 ground；未接地固联组组内刚性、整体可动。表格、详情、按需标记共享 MotionPresentation；方向是 owning Product 坐标，偏置轴不称为世界轴，瞬时自由度不等于关节、有限行程或极限。关闭/选中变化/版本变化清理标记，不参与持久几何或约束。

剩余运动行选择自动显示有效方向；详情直接列出参考对象、平移方向、轴方向/位置和有意义的螺距，长度跟随工作台单位。显示副本以毫米尺度消除 1e-12 以下噪声，不修改原始证据或分类。analysis-guides 拥有独占几何/材质，在模型之后独立清深度覆盖绘制；平移箭头 64 CSS px，旋转弧箭头位于真实轴线处，线宽 2 CSS px，正交/透视 zoom、resize 与 DPR 更新仅改变变换或 uniform。标记不参与模型包围盒、拾取、BVH、导出或历史。

入口：[只读刷新与证据门](../../../services/internal/workspace/assembly_engineering_refresh.go)、[当前页签](../../../web/apps/cad/src/features/workbench/assembly-conflict-panel.tsx)。回归：[真实 Router/Worker](../../../services/internal/control/assembly_engineering_refresh_integration_test.go)、[浏览器与 Viewer](../../../web/apps/cad/browser/assembly-engineering-live.spec.ts)。

## 发布与关联设计

“发布接口”管理稳定 Publication，包括重命名与兼容重定向；“引用设计信息”在已激活 Part occurrence 中创建或复用本地输入，并管理绑定、定位、重连、固定与恢复跟随。新端口与 Product 绑定复用设计事务，预览和取消不创建模型对象。“应用并继续引用”一次提交一项，保留接收草图并更新修订、目录和预览基线；随后取消只丢弃未应用草稿，已应用项可逐次撤销。Part 发布与 Product 转发、重定向使用同一选择字段，核对目标文档、修订和完整实例路径；子装配接口可继续转发，定位仍指向确切 occurrence，改名不改变接口身份。参数的既有定义级 ExternalParameterRef 仍作用于共享 Part，保持独立语义。

| 发布类型 | 当前上下文消费者 |
|---|---|
| PARAMETER | 已存在的本地参数，保留 ParameterId、表达式及独立默认 Source；绑定值在产品上下文中只读，默认值在独立 Part 编辑 |
| PLANE / AXIS | 已存在的本地基准，保留 DatumId、名称及独立框架；关联几何在产品中通过关联管理更换来源，独立 Part 编辑默认框架 |
| POINT / CURVE | 目标草图的稳定外部输入槽；原生 POINT、LINE、CIRCLE、ARC 使用已求解几何及支撑框架精确投影 |
| SURFACE / BODY / FRAME | 不开放本轮上下文消费者；已有发布、转发及其他合法用途保留 |

本地槽位身份由 ContextInputId 分配，与 BindingId、显示编号和临时几何无关。原生草图发布保存明确的 SketchId、EntityId、EntityKind；不伪装 BREP 边。每级转发复制来源描述并变换内部支撑框架，外层解析描述与精确投影同属该 Product 坐标系；接收端再应用冻结的实例相对变换，不重复变换二维几何。外部点经现有 Coincident 驱动普通本地 POINT，再用于 Hole；不直接用投影点冒充 Hole 定位实体。曲线复用 Worker 的精确投影及支撑关系，显示折线不参与关联真相。实体直线边保留 BREP 的有限毫米参数区间，圆/圆弧保留有向框架与弧度区间；上下文草图消费者仅开放已有精确描述及投影的曲线，不把无限轴线或尚未接通的椭圆描述当作有限边。

Product 只保存来源和值：ContextBinding 冻结来源 Revision、完整 occurrence 路径、相对变换及发布契约，ContextVariant 由基础 Part 和已接受输入重建。独立打开使用保存的本地默认输入。来源改变、失效或重定向经 UpdatePlan 显示可更新/阻塞状态；完整健康候选才接受，失败保留原已接受结果。已有绑定的改名与固定只使用已接受输入，不要求失效来源仍可选择；重连沿用原绑定和本地槽位，保留下游身份。PINNED 使用确切旧来源与变换，不读取来源 Head；恢复跟随后仍需接受更新。直接重复实例复用同一 Part 端口，各自绑定和投影独立。上下文编辑合同见[编辑会话](product-edit-context.md#编辑会话)。

子装配消费者保存模块的公开 PublicationId 与模块 occurrence 路径，不绕过接口绑定内部布局。各 owning Product 的 UpdatePlan 只更新自身引用和绑定；来源装配内部待更新或受阻时，父计划携带来源计划摘要并保留已接受结果，可从关联管理或更新修复入口转到来源装配。先在来源装配接受内部来源，再在父装配接受模块和本地输入；普通 FOLLOW_HEAD 仍可跟随已计算版本，关联输入保持显式接受，PINNED 输入和固定模块不读取新 Head。接受输入的全部 Binding 快照进入同一 ChangeSet，撤销/重做同时恢复孔位、来源及引用。

“替换组件”由装配工具和直接组件的结构树菜单进入，独立于发布管理。当前只替换 owning Product 的一个直接来源组件，支持 Part→Part、Product→Product；仅对实际被 ContextBinding、当前装配约束和对外转发接口使用的 Publication 指定对应关系。名称只推荐唯一兼容候选，命令冻结装配基线、新组件确切 Revision、旧/新 PublicationId 和影响集合，权威校验类型、兼容版本、契约、量纲及解析状态。实例名称、策略和初始位姿保留；候选求值与求解成功后才可采用，视图区展示候选组件和关联结果，取消恢复原显示且不写 Head。替换不调用整装配 UPDATE_REFERENCES，其他实例和无关已接受输入保持原版本；后台自动跟随也不会接受含待确认组件或关联输入的计划。接口、Binding、本地端口、约束和下游 Feature 保持身份，全部迁移与求解变化进入一次正式 ChangeSet，撤销/重做恢复完整关系。

替换输入按完整 occurrence 路径和稳定实例 ID 取得两端，不依赖实例或 Binding 的排列。接收定义采用 Product 已接受的实例版本，各 Binding 保留自己的已接受来源和值快照。候选在现有求值与装配求解之间最多重算四轮，直到受影响输入的相对变换与最终位姿一致；预览、直接提交与预览采用共用最终检查，不一致时拒绝并保留原模型。只更新本次迁移的输入，其他已接受绑定和待接受版本不变。来源快照在每次新操作或明确重选时重新获取，在本次映射、预览和确认期间冻结；后台刷新不改变候选版本。

首轮不迁移接收组件的本地输入、深层私有引用、原始拓扑/基准支撑或机构关系，不支持批量或跨类型替换；被选来源提供固定输入时要求先明确处理该关联，不能隐式解除固定或改绑。FIX 的整个 occurrence 与现有直接实例固联组沿用实例身份。未使用的接口无需映射；缺失、不可用或不兼容接口、关联几何失败及约束迁移失败均拒绝候选并保留原装配。

实现入口：workspace 的 publication、context_input_geometry、product_context、product_design_transaction、product_update_release、instance_replacement、context_editing；Web 的 design-association-dialog、replace-instance-dialog 与 context-edit-scope。真实回归入口为 control/publication_layout_integration_test、control/subassembly_publication_integration_test、browser/publication-layout-live.spec。它们是验证入口，执行范围以本轮报告为准。

接收 Part 限定当前 owning Product 的直接子实例；来源为直接 Part 发布或直接子 Product 的公开接口。支持逐层公开转发与各层独立接受，不提供任意深层上下文编辑、深层或批量组件替换、跨层原子更新编排、Wire、曲面/实体 Feature 输入或 Skeleton 文档类型。删除发布的跨工作区并发预检限制见[建模与显示](model-display.md#参数与-publication)。

## 显示、历史与发布

新 DocumentView 即使 GeometryKey/pose 不变，也同步 owner/version/referenceMode、InstancePath、选择与操纵绑定；开始手势从当前权威快照按稳定 occurrence 冻结，旧路径严格拒绝。约束显示按 owner/occurrence/ConstraintId 增量 reconcile，增删、模式、抑制、评价、支持与标签独立失效，请求帧并清理拾取/高亮，无 GLB 下载或完整 BVH 重建。单一手柄吸附复用可见拾取与精确查询；中心重定位不是模型移动，手势框架冻结。

SolveManifest 冻结用户定义、编译输入、descriptor/来源、occurrence、参数解析值、模式/激活、组阶段、分支/基准及 policy/profile/build；失败试算和采用结果分别记录在操作诊断与 SolveManifestResult。Replay 不查询最新几何、不依赖 Session。开发数据只支持当前 DefinitionVersion=2、Quantity 和 assembly-m4m5-reference-retraction-v13/canonical JSON 摘要；旧实验定义、Offset 符号约定、pair-Rigid 持久记录和 policy 明确拒绝，不做读取修补、换号、组迁移或多代投影。Rigid 快捷创建仍生成当前正式组，内核刚性原语继续使用。已有旧开发数据可能无法读取，未自动重置。当前格式的 JSONB 数值规范化、篡改拒绝、Revision、CAS、Undo/Redo 与 Release 冻结完整保留；记录实际重放 build，不承诺跨版本逐位一致。

## 入口与限制

- services/internal/workspace：product_context、product_design_transaction、product_update_release、assembly_solve、assembly_interaction、assembly_interaction_continuation、assembly_conflict、assembly_engineering、assembly_solve_manifest。
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
