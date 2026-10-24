# 当前机构、命令试动与录制模拟

> 根 owning Product 支持 Ground、旋转、棱形、圆柱、平面、刚性与球面接合，使用统一约束求解链、稳定坐标及完整驱动向量进行试动、录制、回放和独立产品转换。本文描述代码合同；验证范围与尚未覆盖的目标须分别判断。参考行为来源见 [CAD 工作流资料](../../references/cad-workflows.md)，当前语义以本文及源码为准。

## 应用、定义与位姿所有权

“装配设计 / 机构与 DMU”属于同一 owning Product。明确选中机构时“机构”编辑该对象，否则新建，不默认采用首个已有机构。当前会话新建且没有确认定义或移动的空机构在退出时通过正式命令删除；已有空机构保留。树中机构及模拟中的机构引用使用同一稳定 Mechanism ID。

Product instance.pose 与 Mechanism.poses 独立。机构只保存自己的成员位置及接合连续坐标；完整 ApplicationDisplay 合并机构成员与非成员环境。进入从 accepted 位置启用机构关系，忽略 Product 方程；退出从机构位置启用 Product 活动关系。没有产品关系时保留并持久采用当前位置，有关系时允许重新求解。退出仍要求最终位姿的独立几何复核；圆柱面接触按材料侧及内／外接触核对轴线平行、径向距离或同面条件，不以数值收敛代替复核。失败保持当前应用和已接受位置，通过统一通知反馈。

`mechanism-application` 只读准备当前 Head 的候选；`ACTIVATE_MECHANISM` / `EXIT_MECHANISM` 经重新验证、CAS 和 Undo/Redo 采用。空 ChangeSet 不产生 Revision。有序 sourceMechanismIds 只在匹配 Head 的应用环境中使用，不让机构持有非成员关系。URL 记录应用、机构和来源 Revision；刷新重新验证。外部 Head 变化撤销旧预览和采用资格，使用当前文档环境，不能把旧入口帧补写回去。

Viewport Engine 原子提交完整 occurrence 帧。请求开始、拒绝、迟到或预览资源清理不先显示旧 Product 原位；取消显式恢复入口 accepted。自有提交的 Query 刷新由完成屏障保护，显示帧与 Revision 配对后才交接。

## 树、支撑与机构评价

```text
Applications
├─ 机械装置
│  └─ 机械装置.1，自由度=n
│     ├─ 接合 → 各类接合 (Part.1,Part.2) → 所属真实约束／固联组
│     ├─ 命令 → 命令.1
│     └─ 固定零件 → 固定零件.1 (Part.1) → 固定.1
└─ 模拟
   └─ 模拟.1
      ├─ 源机械装置的引用投影
      └─ 结果.1 · 完成 / 部分结果 / 未通过求解
```

机械装置定义工具类别将各类接合平铺显示，不再使用接合下拉分组。

接合下的关系是真实独立 AssemblyConstraint，复用普通约束名称、图标、选择、高亮和所属 Joint 编辑。名称及编号不是身份。产品与机构约束不自动同步，转换生成独立身份。树投影包含 ownerEntityId、mechanismId、relationRole；Inspector 只读。录制草稿只增加本地树投影，取消后消失。

固定件、刚性接合和多实例化接受视图区或树选择，按 owning Product 解析所属直接 Instance，并整体高亮。轴允许基准轴、直边及精确可解析的圆柱面、圆边、圆锥面；点允许基准点、精确顶点及显式派生的球心／Frame 原点。轴与面属于对应端实例。棱形／圆柱的 AxisDirection 控制轴关系 SAME/OPPOSITE；偏移与平面方向 AxialDirection 独立设置，两者都不复用坐标 Direction。棱形的平面方向同向／相对分别以定向角度及定向角度加 180° 编译；预览、持久关系、闭合见证和产品转换使用同一分支。支撑引用持久化为 InstancePath、Publication / PersistentSelection；派生 Frame、CapturedX 和接合零位用于坐标，不从 mesh 下标猜测。两端横向基准分别从各自 Part 的局部方向捕获，不从另一端当前世界方向或世界 X 临时构造。

| 接合 | 真实关系 | 稳定坐标 |
|---|---|---|
| Ground | 捕获的 SPACE Fix | 无 |
| 旋转 | 轴重合＋有向轴向平面偏移 | AXIAL_ROTATION / ANGLE |
| 棱形 | 轴重合＋绕第一轴的投影定向角 | AXIAL_TRANSLATION / LENGTH |
| 圆柱 | 轴重合 | 独立 ANGLE 与 LENGTH |
| 平面 | 两平面重合或有向偏移，含 SAME/OPPOSITE | 无驱动坐标 |
| 球面 | 两个精确点重合 | 无驱动坐标 |
| 刚性 | 一个两成员 FIX_TOGETHER | 无 |

机构预览、定义确认、试动和路径共用 FreezeAssemblyInput 与生产 Worker。闭合检查分别验证同轴、定位角、有向平面关系、点位置或相对 SE(3)，不统一套用旋转副的原点与 Z 轴检查。新建棱形接合先以既有关系及轴重合构造合格位置，再在该位置编译完整定向角，避免从退化的投影方向冻结角分支；只有完整关系通过独立硬见证及偏好门禁才可确认，构造步骤不形成 Revision。取消仍恢复编辑入口。

刚性首次定义先求解保留的机构关系，再捕获两个成员的相对位姿；普通改名、移动、播放与其他接合编辑复用原捕获，不调用产品组的自动内部更新。没有 Ground 的刚性对保留六项整体运动。刚性子组的成员树只是引用，编辑委托 owning Joint，不能单独删除成员或重定义相对关系。

Mechanism.evaluation 保存 physical relativeDOF、gaugeDOF 和候选 Driver 数量。保存多个命令不会把它们同时加入硬方程；树显示物理相对自由度。试动检查物理机构 gauge=0、相对自由度大于零，以及活动坐标方程加入后相对自由度与 gauge 均为零；独立秩减少量须等于活动坐标数。不以“自由度减命令数量”代替求解，也不要求驱动接合直接连接 Ground。会话可以处于待资格状态，允许用户选择完整集合；仅存在唯一合格基时自动选择，候选超过有界枚举范围则等待显式选择。

每个 JointCoordinate 的 Lower/Upper 是唯一限位，使用稳定连续坐标。未驱动坐标可独立启用任一侧；驱动坐标必须两侧均启用且有限，下限严格小于上限。UI 在接合与模拟中按当前命令零位统一显示，u_min/max = q_min/max − q_ref，只派生显示范围而不保存第二套范围。缺失端在勾选驱动时提供角度 ±360°、长度 ±100 mm 的显示默认值，既有限值保持。已知当前位置越界或范围无效时显示短状态并阻止确认；服务端通过完整求解和全部坐标限位检查拒绝无效候选，不偷偷调整位置。非活动坐标限位也约束数值输入、空间操纵和全部路径样本。命令被模拟引用时禁止撤销；显式删除相关模拟后可以撤销，单独删除命令不会级联丢弃记录。

## 稳定命令零位与统一数值引擎

JointCoordinate 保存稳定 ID、角色、ANGLE/LENGTH 维度、Direction、物理 Zero 与限位；Zero 在接合定义通过后捕获，不能由重开对话框改写。MotionDriver 精确绑定 JointID 与 CoordinateID，保存 Reference 和 ReferenceDigest。接合的“角度驱动／长度驱动”勾选分别创建命令，取消勾选撤销未被模拟引用的命令；相同 ID 不能改绑接合或坐标。命令没有独立创建入口或范围字段，编辑提供所属接合、参数、只读当前读数和“置零”。创建命令或明确置零时，在合格构型捕获一次 q_ref；操作坐标为 u=q−q_ref，硬驱动目标为 q=q_ref+u。再次打开不重新捕获零位。Mechanism.coordinates 按 CoordinateID 保存完整坐标，角度连续展开、长度不取模，跨保存、刷新保留分支。

坐标基准摘要排除显示名、数组次序和物理限位；修改限位不偷偷重设命令零位。录制基准摘要包含全部机构关系、Ground、限位和驱动坐标绑定，几何摘要包含成员解析 Revision 与几何制品身份。接合坐标基准改变后须明确重新设零；几何／物理关系改变的旧记录须重新验证。单纯命令置零不改变接合坐标基准或物理挡位，录制基准摘要排除命令读数原点。已保存关键帧继续由各自冻结 Reference 解释；打开编辑时以 q_ref_old + u_old − q_ref_current 换算一份私有草稿，保持 ID、时间、分支和实际路径，确认前不改原记录。

`mechanismMotionSolver` 服务于完整命令向量、空间 DragTarget、录制路径。所有路径保留机构全部硬关系及固定基准。数值目标加入各活动角／长度方程；长度使用第一端冻结 Z 的有向 point–plane 距离。空间操纵采用现有软目标和偏好资格，不保留上一驱动向量作永久锁；结果回算全部坐标后再次探测活动集合资格，恢复空间目标的操纵证据。nominal 保持冻结，initial_guess 和分支随合格检查点推进。输入摘要包含当前装配求解版本策略；Worker 回传版本必须匹配该策略，升级后不能复用旧路径的计算身份。

混合向量按冻结的角度步长（5°）与长度步长归一化选择共同子步，并有界细分；检查完整位姿、独立硬残差、逐类闭合、全部限位、连续角和驱动自由度。全部成员的相邻刚体转动超过 90°或位移超过十倍冻结长度步长时转入同一细分，覆盖没有标量角度的被动连接；这只是路径连续性门禁，不增加姿态硬锁。已知活动目标越界在继续求解前拒绝，沿途被动限位仍逐步检查。最终目标失败时恢复请求前的全部私有 state，包括 initial_guess、完整坐标、活动集合和分支，不发布内部成功子步。数值路径可以接受已有内核合同下硬可行、独立见证成立的 MAX_ITERATIONS；这不声称偏好阶段收敛。空间目标和最终定义提交仍分别执行其偏好门禁。

## 试动和录制会话

`POST /documents/{id}/mechanism-sessions` 冻结 owning Revision、机构、所选命令、完整环境、几何和 solver profile。会话按 actor / document / 对话框 owner / Session / 输入摘要隔离，采用递增 generation 和 sequence。打开前保留代次屏障；重复挂载、取消及迟到 Begin 不能重新创建旧拥有者。更新在私有引擎副本中计算；只有匹配当前会话、序号、Head 的完整合格帧可以发布。会话有 15 分钟滑动租期，前端每分钟续租；单 actor 最多 4 个、全服务最多 32 个，每份快照不超过 8 MiB。

浏览器的 MechanismMotionSession 同时拥有完整数值向量、游标、空间手势和录制。最多一个求解请求在途，后续目标合并到最新完整向量；已完成合格帧先发布，再处理后续输入。Pin、有限路径预览和最终确认等待输入屏障。试动中的活动集合切换从同一合格构型测量各坐标并探测独立资格，保留全部物理方程和连续角；成功才更新集合、冻结 References 与输入摘要，失败保留原集合及模型。界面提供“使用”复选框，活动行可输入，其他候选只读回算。录制 Pin 起点后冻结活动集合。序号／摘要不匹配、取消及旧 Revision 响应不能覆盖新位置。

- **使用命令进行模拟**：不创建模拟定义、结果或 Job。各坐标、游标和模型只表示同一已发布合格检查点。失败返回 Accepted=false 与原完整帧，简短提示并允许下次输入。确定为当前位置准备正式预览候选，再提交一次 SAVE_KINEMATICS；取消、Esc、标题关闭恢复入口位置。
- **模拟**：命令窗口与录制窗口共用上述拥有者。建立合格驱动集合后自动 Pin 入口 t=0；待资格状态不生成起点。提供插入、修改、删除、自动插入、动画视点和时间定位。关键帧有稳定 ID，保存未四舍五入的时间、以 DriverID 为键的完整带单位 Values 向量和可选相机。Study 冻结 ActiveDriverIDs 与各命令 References，不从对象迭代顺序推导绑定。起点不能修改或删除。自动插入只接纳已发布合格输入；时间定位不再自动录成新关键帧。
- **录制确认**：服务端对关键帧 ID / 值核对 Pin 的合格见证，填入来源 Revision、零位、初始完整成员构型、连续坐标及冻结非成员环境。主窗口一次准备与提交 Simulation 和当前合格位置，形成一个 Undo 项；两个窗口没有独立持久写入。只有起点的记录也能保存。

自动插入在操纵输入时冻结逻辑时间和可选视点，合并请求保持最新输入的原时间，不使用服务器答复时刻。逻辑时钟按相邻输入间隔推进，步长为 0.001–0.2 s，首次或显式调整起点后为 0.2 s；空闲及求解等待不会无限拉长动画。只记录合格且坐标／视点有变化的状态，最多 500 帧、3600 s，达到容量时停止自动插入而保留试动。手动插入和修改使用指定时间；修改保留关键帧 ID。

共享会话中的“移动组件”先在精确逻辑时刻暂停／求解，再取得空间操纵所有权，只调整试动检查点，不在 pointerup 创建 Revision。已绑定当前 Head 的合格录制会话允许启用该入口；历史结果和纯显示插值不取得编辑资格。操纵器仍使用已有 ToolManager、AssemblyInteraction 和结束屏障；pointerup 后保留手柄。取消空间手势重新请求手势入口完整向量；取消主窗口撤销整个会话。普通机构／Product 操纵保持一次正式提交和原目标资格，复用既有候选及回执恢复通道。普通机构移动的候选同时采用合格矩阵和接合连续坐标，再执行最终机构一致性校验；不混用移动前的坐标与移动后的矩阵。未激活的机构面板不清理全局选择；相同语义选择的显示资源重新绑定属于结果恢复，不取消已提交手势。

双击保存的模拟按其冻结初始构型进入编辑。定义或成员几何摘要不一致时拒绝混用；“采用当前基准并验证”明确沿当前定义验证原关键帧值，保留 ID、时间和顺序，确认前不修改文档。失败保留原记录，不声称整个机构无解。

会话端点为 Begin、updates、keys、preview、finalize、touch、DELETE。只读用户可做暂态计算和查看；finalize 与领域提交要求 Editor。会话取消释放计算及候选，不写 Revision。finalize 使用现有正式 PreviewCommand / PreviewID，客户端提交原 CommandRequest；没有新建跳过 evaluator 的保存通道。正式提交只复用服务端 Pin 的内部见证；直接领域命令修改录制坐标时重新验证初始构型和关键帧，客户端不能自证求解通过。

## 机构分析

“机构分析”可从 Product 或机构工具栏／搜索打开，默认当前机构，也可明确选择其他已保存机构；切换查看不激活机构。分析数据绑定 owning Product、Revision、Mechanism、规范输入及 Naming/solver 策略。界面首屏显示相对自由度、整体运动及待处理问题，接合列表显示名称、关联零件与工程状态，选中后查看支撑、真实约束和全部命令／坐标。逐项保留缺失、歧义、类型不符及其他解析诊断，区分支撑、当前构型检查与数值结果；基础设施错误显示失败，可取消、重试。打开／切换自动分析，复用当前版本缓存并合并在途请求；不提供 JSON/CSV 导出，内部来源与策略仅在DEBUG开发信息面板按需查看。

只读刷新使用该机构完整接合、Ground、确切已接受引用和 `Mechanism.poses`，复用原支撑解析器、冻结编译、Jacobian/rank 与剩余运动证据门，不导入 Product 约束、不按 Product 的 Verified 状态过滤、不施加命令锁。相对 DOF 与整体 gauge 分别报告；数值锚点不是 Ground。输入不完整、求解失败、返回位姿或 linearization pose 离开冻结构型时保持未知，不采用求解姿态。命令表只说明绑定，试动资格仍走既有 Session 流程。

分析不写位姿、命令零位、连续坐标、刚性捕获、生产 warm start、正式 Session 或历史；Viewer 可使用。结果保存在可回收的前端查询缓存，当前版本重开可复用，版本变化需重新分析，取消与迟到响应不能恢复旧结果。行选择定位树与参与实例；“显示接合”复用原关系 renderer，方向共用 MotionPresentation。视口不在分析构型时隐藏分析标记，不为显示移动实例。

失效机构无需先激活即可从报告进入原 Joint 编辑、支撑重选、权威预览和 `SAVE_KINEMATICS` 确认链；修复上下文临时存在，取消恢复显示且不写 Head，成功保留机构／接合／命令及关系身份。完整候选仍需沿原链成功，不丢弃其他失败接合来放行修复。本入口限当前 owning Product 的已保存七类接合，不分析实时试动／回放、路径可达性、规律、dressup 或柔性机构。

入口：[后端分析](../../../services/internal/workspace/mechanism_analysis.go)、[分析视图](../../../web/apps/cad/src/features/workbench/mechanism-analysis-panel.tsx)、[原 Joint 编辑](../../../web/apps/cad/src/features/workbench/motion-study-panel.tsx)。定向验证：[真实 Router/Worker](../../../services/internal/control/mechanism_analysis_integration_test.go)、[motion-live 浏览器](../../../web/apps/cad/browser/motion-live.spec.ts)。

## 有限路径、回放和结果

MotionStudy 只接受录制的 Keyframes，不存在规律对象、规律引用或起止值扫描分支。初始完整构型、全量坐标、活动集合与每个命令零位冻结路径起点；按时间分段线性生成完整操作向量，用同一引擎求解全部硬关系、分支及限位。采样时间包含配置的均匀网格和每个精确关键帧时刻。关键帧本身合格不作为完整路径通过的证明。MotionSnapshot Schema=2，求值策略为 joint-coordinates-vector-recorded-path-v3；帧的 Coordinates 以 CoordinateID 为键，CommandValues 以 DriverID 为键。未发布数据只使用唯一向量模型，不保留旧标量录制分支。

录制内播放先通过私有 preview 计算有限路径，不创建 Job。结果是合格样本缓存；失败保留之前样本及失败时间。后台 MOTION_STUDY 只负责已保存有限路径的制品生成。任务输入冻结 view、Study、机构、参考零位、环境、方程及版本策略，计算中不读变化的 Head。Job 的 attempt / lease / cancel / SHA 合同保持。

冻结 View 保持 owning Revision 的 Product 与 resolvedInstances 成套一致。机构 nominal 和冻结非成员环境只用于数值输入／方程，不改写显示文档的 Instance 位姿。回放通过完整帧覆盖既有 occurrence 根节点；内部 Body 保留相对于源 Product 的局部变换，避免重建时额外施加旧位姿。浏览器验收同时检查 Instance 根节点与实体世界矩阵，不能只用根节点位置证明显示正确。

Job SUCCEEDED 表示结果保存成功；payload.motion 保存实际运动学 status、completed 和 validFrames。树按路径质量显示完成、部分结果或未通过求解。每份结果按稳定 Job ID 挂在自己的模拟下，双击查看指定结果；没有全局结果选择、播放工具栏或对话框删除按钮。终态结果右键隐藏用户列表项，保留审计与 Artifact，不写文档 Revision；运行中的结果先停止。

播放用统一 RAF 时钟更新既有 occurrence，在相邻合格完整样本之间作刚体螺旋／四元数插值。时间显示两位小数，但逻辑时间保持精度。显示插值不能作为闭环物理正确性的证明，也不具备录制、保存、转换或测量资格。视点插值独立于方程；切换动画视点不重新计算机构路径。

录制内暂停／时间定位按精确逻辑时刻生成操作目标，再通过共享会话求解；失败退回上一个合格位置及时间。结果采用与转换通过 motionPlayhead 从冻结起点沿同一路径求解精确时间，不取最近样本。当前 Head 的结果“确定”经 `motion-pose` 准备正式候选再提交；旧 Head 结果确定只结束查看，历史位置转换须针对当前定义和产品方程重新验证。取消恢复机构 accepted，关闭前台资格后，迟到结果不抢占视口。

## 产品约束转换与错误合同

转换统一从工具栏／搜索进入。试动与录制必须先确定或取消；结果转换暂停并固定精确时间。取消返回同一回放位置与逻辑时间，成功退出装配设计。对话框采用标准 M 宽度及确定／取消。

来源为当前机构 accepted 或不可变 Job + 合格 frameIndex / 精确 timeSeconds，二者互斥。按 JointID 与稳定 RelationRole 转换各类真实关系：Ground 生成独立 Product Fix，刚性生成两成员产品固联组，其余复制所属重合、偏移或定向角关系。默认保留运动自由度；显式 lockCoordinateIDs 可分别增加角度／长度静态锁。锁的来源为约束自己拥有的 captured body-local Frame，保存稳定来源、SourceVersion、有限右手 Frame、mm 单位及策略，通过正式支持解析；不依赖临时 Worker geometry ID，也不修改被引用 Part。长度锁使用有向 point–plane 行程，不能用同轴线间距离替代。只允许 ADD 或定义一致的 REUSE，差异是 CONFLICT，不更新、停用、替换或删除原产品关系。目标删除后分配新身份。

转换验证当前来源语义、稳定支持、几何／基准、全部 Product 活动方程及所选位置。非成员采用当前产品位置，历史冻结环境不覆盖后来的产品变化。一次 CAS 保存新增关系、变动位置、计数器和映射，一次 Undo/Redo 恢复。

目标越界／局部数值失败属于可恢复反馈，不每次弹窗。服务端归档实际 MotionInput、不可变 MotionSnapshot、最后发布检查点和生产 Worker replay；复制的是 `CAD_DIAGNOSTIC document/id`，归档不可用时不伪造可重放命令。取消、预算耗尽、输入错误、执行故障和旧 Head 分别处理。网络失败后确认重新验证原目标，仍提交同一已经准备的 CommandRequest，避免另建移动意图。

## 范围及继续扩展的边界

当前范围为根 owning Product、刚性实例、上述七类接合和单机械装置内完整独立驱动集合。圆柱可同时驱动角度／长度；平面与球面为被动接合，可参与由其他接合驱动的混合机构。非 Ground 端的接合可以驱动，未选候选命令不成为额外硬锁。未接地、欠驱动或冗余集合可以保存定义，但须通过真实资格检查才能录制。

尚未交付多机械装置同步、齿轮／螺旋等耦合接合、可动嵌套 Product、动力学、连续碰撞或机构约束来源导入。路径目前按整份有限输入重算，尚无分段制品复用、曲率／显示误差自适应采样或独立的段质量持久模型；失败时间和合格前缀由 Run 表达。单会话预算与资源上限是保护，不是工业容量、P95 或帧率结论。

主要入口：[会话服务](../../../services/internal/workspace/mechanism_session.go)、[统一求解](../../../services/internal/workspace/mechanism_motion_solver.go)、[录制模型](../../../services/internal/workspace/motion_recording.go)、[冻结路径](../../../services/internal/workspace/motion_snapshot.go)、[精确定位](../../../services/internal/workspace/motion_playhead.go)、[浏览器拥有者](../../../web/apps/cad/src/cad/assembly/mechanism-motion-session.ts)、[双窗口](../../../web/apps/cad/src/features/workbench/mechanism-motion-dialog.tsx)。真实验证分别由邻近数值／SQLite/CAS/历史测试、Web 状态场景及 [浏览器验收](../../../web/apps/cad/browser/motion-live.spec.ts)承担，不能互相冒充。
