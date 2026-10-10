# 机构与 DMU 应用及仿真转装配

> 返回[当前架构目录](../../CURRENT_ARCHITECTURE.md)。这里描述代码合同；复现、演示及尚未验收项见[实施与验证记录](../../../plans/kinematics-dmu.md)。更丰富的关节、仿真和碰撞流程仍在[目标合同](../target/kinematics-dmu.md)。

## 应用、定义与几何身份

同一根 Product 通过唯一 `workbenchconfig/catalog.json`、`contexts/tabs/placements/groups` 提供“装配设计 / 机构与 DMU”分类，沿用 `WorkbenchCommands` 的唯一 Tab 区、文档会话选择和搜索；不另建 Tabs/命令按钮目录，不创建 DMU 文档。应用会话由选中的配置 Tab 决定，独立于单次命令；定义/转换走统一 `CommandDialog` 与 form 的 `CommandOperation`，运行、回放和取消为 handler。CommandRegistry 提供新建机构、固定件、旋转/移动/刚性接合、驱动、研究、试动、干涉分析、导入和应用到装配等入口。Applications 树投影 Mechanism、Joint、MotionDriver、MotionStudy、InterferenceAnalysis、只读来源/展开项/发布映射，以及当前用户任务列表中的只读运行节点。树选择通过稳定对象 ID 映射完整 occurrence + Body 高亮；重命名、编辑、删除走正式命令，不直接改树。

Product 的 `kinematics` 保存独立定义；运行结果是 Job/Artifact。允许保存空机构、暂未接地的机构及逐步添加的接合。MotionDriver 引用 Revolute/Prismatic 运动坐标，MotionStudy 必须引用 Driver，保存线性时间规律；干涉范围、间隙和同刚体内部检查保存于 InterferenceAnalysis，可独立检查当前姿态或关联 Study。单坐标试动可直接引用 Driver，不要求先保存时间研究，生成冻结的临时两帧研究而不增加定义 Revision。研究中的 driverJointId、DMU 设置是冻结运行派生值，不接受作为另一套持久定义编辑。删除机构/接合联动清理依赖驱动、研究和映射；被删除研究的分析解除研究关联而保留分析。删除接合或解除来源/发布关联不删除正式 Product 约束。

一个 Mechanism 属于一个 owning Product；直接子 Part（包括多 Body Part）和子 Product 是刚体运动单元。子 Product 内部相对位姿冻结，未加入机构的直接子实例固定在研究基准。当前应用在根 Product 编辑上下文开放，不支持可动嵌套机构。

新接合通过两个真实轴支撑和两个定位平面拾取，复用装配 PersistentSelection、InstancePath 和 Publication 解析。圆柱、圆/圆锥可作为轴，平面必须垂直于对应轴。端点轴与平面交点及正交基生成局部 Frame（毫米、x/y/z/w 四元数）；几何引用是定义，Frame 是派生值，运行前重新解析。两定位平面不定义角度零位：定义显式保存两端体局部 CapturedX，创建时捕获共同世界横向基准；解析时由其构造与支撑轴正交的基准，退化时拒绝。零位、±1 正方向、限位和轴向偏移使用有单位 quantity。`q=(raw-zero)/direction`；Revolute 使用第一端 Z 的连续角，Prismatic 使用沿第一端 Z 的有向位移。Ground 固定研究基准姿态。既有数值 Frame 定义仍可用于闭环研究，但没有持久轴/平面支撑不能发布成正式几何装配约束。

`SAVE_KINEMATICS` 适配版本化 `occccad://product/kinematics/set`，仅修改 `product.kinematics` PropertySlot，必须携带当前 versionId，使用已有幂等事务、Head/sequence CAS、ChangeSet、Outbox、Undo/Redo。不改变正式约束或位姿；保存时解析新增/改变的几何接合，运行时只检查所选机构及其依赖。无关草稿或其他机构的失效引用不阻断所选研究。定义合法不代表研究可运动，DOF 与硬方程门禁在运行时检查。

## 与正式装配的关系

机构只求解自身 Ground/接合和显式采用的 `supplementalConstraintIds`，不自动继承全部 Product 约束。`GET /api/documents/{id}/motion-joint-proposals` 从 SPACE Fix 和稳定轴相合＋定位平面偏移生成 Ground/Revolute 提案，用户确认后导入并保存来源 ID 与定义基线。来源是引用，接合下的轴相合/轴向定位是只读语义展开项，不是两套可独立修改的真相；已作为接合来源的关系不能又作为补充方程加入。

来源定义变化/删除在树中分别标 SOURCE_CHANGED/SOURCE_DELETED，不静默更新接合或删除原约束；用户可明确重新导入或解除来源关联。它与几何失效分开：来源变化不自动改运动定义，选中机构的几何解析失败则阻止运行。发布映射保存 Joint 与正式 Constraint 的基线；正式关系或接合有变化时转换计划做差异及冲突判断，不使用 solver 私有 ID。

## 完整硬方程与帧判据

复用生产装配输入冻结/编译器和数值内核；机构编译把派生或显式局部 Frame 转为同一内核的 Point/Axis/Plane descriptor。所有显式采用的活动、未停用、非 Measured 补充装配定义必须有方程，缺失时拒绝运行。已捕获的 Fix Together 展开为硬刚性方程，捕获未完成或试图改变其 Relative Fix 基准时拒绝。

| 关节 | 硬方程 |
|---|---|
| Ground | 基准世界 Pose 的 FIX |
| Rigid | 原点重合、Z 同向、X 同向 |
| Revolute | 原点重合、Z 同向；自由坐标为绕 Z 的连续相对角 |
| Prismatic | Z 同轴、X 同向；自由坐标为沿 Z 的有向位移 |
| 单坐标驱动 | 有参考 Z 轴的 ANGLE，或点至平面的 ALONG_SECOND_NORMAL DISTANCE |

保留全部接合、闭环和显式补充方程，不树化删除闭环。MotionStudy 可明确指定 `replaceDriverConstraintId`；只有同端点、同有向局部几何坐标的独立 ANGLE/DISTANCE 方程能被本次运行的硬驱动替换。组约束、不对应的坐标和未知 ID 均拒绝；替换只作用于冻结方程，不停用或修改 Product 约束。编译记录 retained/replaced constraint IDs，驱动不是 DragTarget 或软偏好。

首版要求接地后 relative DOF=1、gauge DOF=0；加入每帧驱动后 relative DOF=0、gauge DOF=0，否则报告 UNSUPPORTED_DOF。当前固定完整 nominal，只用上一合格帧更新 initial guess 和已接受的 angle/alignment/distance branch。研究接纳与普通装配编辑的成功语义分开：硬约束合格、FIX 独立检查、全 Joint 闭合和驱动坐标验证后才保存帧。普通 CONVERGED 或硬可行但偏好未完成的 MAX_ITERATIONS 可以进入研究门禁；后者还必须通过现有完整独立硬方程见证。没有硬见证则失败，不修改普通装配偏好优先级或容差。Joint/驱动位置门限 1e-7 mm（角坐标 1e-7 rad），轴方向门限 1e-8；装配方程继续使用冻结 solver profile 的单位和归一化残差标准。

规律为 `q(t)=start+(end-start)*t/duration`，结果存规范 rad/mm 和连续角度，驱动 RPC 的周期角度与保存的连续坐标/分支分开。驱动角大步先细分至最多 π/12，其他失败有界二分重试；最多深度 10、2048 次 solve，全部共享研究 deadline。非驱动角的过大分支跳变会触发细分。每个合格 continuation 子步都检查关节限位；限位与扫描 start/end 独立，遇限停止，保存此前采样帧。DMU 只在指定离散采样帧执行，隐藏 continuation 子步不是检查样本。

KINEMATIC_FAILED 表示本次预算和初值未得到合格帧，不证明无解；INPUT_ERROR、EXECUTION_FAILURE、UNSUPPORTED_DOF、LIMIT_REACHED、BUDGET_EXHAUSTED、CANCELED 与 DMU_UNFINISHED 分开。失败保留上一批合格帧、最后 Solve 诊断，并通过现有 ASSEMBLY_REPLAY 制品保留可用的数值请求。

## 精确 DMU

检查单元与运动单元分开：每个检查单元保存完整 typed InstancePath + BodyId、源 GeometryKey/GeometryId、B-Rep 制品引用及相对 owning 刚体的 Pose。后代几何始终应用所属刚体整帧 Pose；相同 GeometryId 的不同 occurrence 不合并身份。Scope 是明确的 occurrence + Body 集合，空集合表示全部；全部范围中缺少精确几何的运动单元会拒绝检查，DisplayFallback 不用于判定。保存的 Scope 仅 owning root Revision 可重绑定到新定义 Revision，后代 accepted/resolved Revision 必须匹配。默认不检查同一刚体内部 Body 对，可明确开启。

多输入 B-Rep 通过既有 Geometry client 的递归 Artifact staging、Router 和 `AnalyzeInterference` RPC 闭合，S3 输入物化到独立可清理的本机 scratch；Worker 不读取数据库。每次 RPC 使用隔离 OcctKernel，相同制品只加载一次，各实例 Pose 分开。非破坏性布尔和深拷贝变换不修改共享源几何。当前仍串行占用 Worker 的 OCCT 锁，等待锁可取消；OCCT 距离/common 使用进度取消检查，输入读取/有效性检查只在阶段边界响应取消。

保守 AABB 按 max(clearance, contact tolerance) 外扩；它筛掉不可能相交的 **Boolean common**，每一指定 pair 的最小距离仍由 BRepExtrema_DistShapeShape 精确计算，不能将包围盒间距冒充最小距离。实体必须经 BRepCheck 合格、具有正有限体积且不混有游离曲面/边/点。BRepAlgoAPI_Common 的完成、错误、形体有效性和体积均检查。

| 分类 | solid-dmu-v1 判据 |
|---|---|
| SEPARATED | 无显著 common volume，最小距离 > 1e-7 mm |
| CONTACT | 无显著 common volume，最小距离 ≤ 1e-7 mm |
| PENETRATION | common volume 超过体积门限，且不等于较小实体的整体体积 |
| CONTAINMENT | common volume 在体积门限内等于较小实体的整体体积（相同实体重合也归此类） |
| INCONCLUSIVE | 几何无效、输入/计算失败、取消或未完成；complete=false、clearanceSatisfied=false |

体积门限为 `max((1e-7 mm)^3, min(Va,Vb)*1e-12)`；判定受 B-Rep 自身公差及 OCCT 数值精度限制，不声称数学精确或小于该门限的穿透可被稳定分类。间隙以精确 `distance >= clearance` 判定；穿透/包含始终不满足。零间隙 CONTACT 可满足距离要求，但保留 CONTACT 分类。结果含距离 mm、common volume mm³、witness、complete、diagnostic、commonTested 和 kernel build。Worker 个别输入失败可保留其他 pair 的已完成结果，整个报告不通过。Go 检查结果数量、身份、有限度量、witness、分类、间隙及完整性元数据。

帧的 kinematicValid 与 dmuConclusion 独立。完整报告有穿透/包含或间隙不足为 VIOLATION，全部满足为 PASS，任何 pair 未完成为 INCONCLUSIVE；关闭检查为 NOT_CHECKED。干涉帧保存供回放，不让运动学“合格”掩盖干涉。当前姿态静态分析不执行运动学求解，因此明确标为运动学未检查。离散 PASS 仅覆盖指定范围和指定帧，不代表连续无碰撞。

## 冻结运行、取消与回放

`POST /api/documents/{id}/motion-runs` 校验 viewer 权限及 baseRevisionId，冻结 schema=1、policy=grounded-single-coordinate-explicit-relations-v2 的完整 view、关节/规律/方程、solver profile 和 exact geometry refs；冻结前后检查同一个 Product Head。内容 digest 绑定输入，MOTION_SNAPSHOT 制品进入现有 MOTION_STUDY Job；actor/document/requestId 幂等，同 requestId 不同输入返回 409。Job 执行只读冻结输入；中途 Head 变化不混入新版本，也不推进任何 Head。

运行结果为 MOTION_RUN 制品；Job 的 SUCCEEDED 意味着结果已保存，研究自身 completed/status 和各帧结论决定实际质量。研究停止、数值失败、限位和部分 DMU 都可保存为可查看结果。FinishMotion 原子核对 RUNNING、lease owner、attempt 和未过期 lease；取消请求决定 CANCELED 终态，保存已完成帧，迟到 attempt 不覆盖结果。已有取消监视器取消 RPC，完成制品保存使用有界 WithoutCancel 上下文。排队即取消尚无帧；进程崩溃后未持久化的内存帧不能保证恢复，租约重领重新从冻结基准运行。

`GET /api/jobs/{id}/motion-run` 校验任务访问和 Product viewer 权限，并检查结果制品大小、种类和 SHA；历史版本结果可只读获取。已终止研究不在同一 Job 上手动 Retry，重新运行生成新 Job，保留旧结果；保存之前的基础设施自动重试及租约重领继续沿用现有队列。当前列表沿用用户最近任务列表，最多 100 项，无结果分页索引或独立共享研究库。

Web 的定义保存走 Domain Command；补充关系选择与解除发布关联先修改本地草稿，确认时一次保存，取消不写 Revision。视口下方 `WorkbenchLayout.activity` 显示机构/研究/分析/冻结运行选择及整帧回放，属性面板保持只读。任务列表由 TanStack Query 获取，草稿、播放、定位为暂态交互。关闭命令不退出应用；应用退出、上下文/Revision 变化清理播放/高亮及前台任务跟踪，代际门禁忽略迟到响应。后台 Job 不绑定面板生命周期，只有用户明确取消才请求取消任务。旧结果只能显式加载并显示冻结版本标识。历史回放的树、检查器、几何与选择使用同一冻结 view，禁止编辑冻结树。回放使用整帧所有 owning unit Pose；视口拒绝不完整加载的帧，立即批量设置、不逐零件插值。播放中限制编辑，退出或“恢复正式姿态”恢复当前正式 view，不逐帧写 Revision；选中合格帧可通过下述正式转换命令采用姿态。结果行按完整实例 + Body 高亮，问题帧按钮跳至已保留的干涉/未完成帧。

## 应用到装配

`POST /api/documents/{id}/motion-apply-plan` 要求 Editor、当前 baseRevisionId、Job 与帧索引。服务端验证运行归属、结果 SHA/digest、冻结来源快照、当前机构结构和几何定义、有效整帧及完整 owning-unit 姿态。静态 DMU、无效帧、陈旧 Head 或已变更的来源实例快照不能转换。Joint 的派生 Frame 不被误判为定义变化，数值 Frame 定义仍参与比较。

默认同时发布连接和采用所选帧姿态。Ground 生成/复用 SPACE Fix；Revolute 发布轴相合与有向平面偏移，保留旋转自由度；Prismatic 发布轴相合和捕获方向一致而保留轴向位移；Rigid 再发布轴向定位。只有明确“锁定当前角度”才给 Revolute 增加静态有向 ANGLE，使用持久轴引用和捕获横向基准。运动时间规律留在研究中。新约束经现有 public DefinitionVersion=2 规范化、几何支持和单位校验；没有固定全部运动零件或前端矩阵写回替代关系。

计划列出 ADD/REUSE/UPDATE/CONFLICT、位姿变化和求解状态/DOF。确定性约束 ID 与关联基线避免重复发布；等价已有关系复用原 ID、表达式及正式定义。已存在的定位角、Fix、补充关系和来源/发布两边改动可能冲突，必须由用户明确 SUPPRESS 或 REPLACE 后重新生成计划；不静默删方程。解除旧角锁也需要明确停用/替换，不因本次未勾选锁角自动丢弃。

候选必须包含目标 Product 的全部活动正式方程，独立验证它们在所选姿态满足，随后执行普通正式装配求解并要求 CONVERGED 与完整硬见证；求解结果必须仍落在所选帧（位置 1e-7 mm、旋转 1e-8 rad 门限）。这与研究允许硬可行偏好未完成的判据分开，不改变普通编辑语义或精度。默认未锁角也把所选帧作为正式 nominal/initial pose，所以首次落在该姿态。

`APPLY_MOTION_FRAME` 适配版本化 `occccad://product/kinematics/apply-frame`。提交前重新生成权威计划、核对 digest，候选完整求值后再次验证姿态。Constraint 创建/更新/显式删除或停用、Instance pose、发布映射在一次原有 CAS 事务/Revision/ChangeSet 中提交；失败不提交部分修改。Undo/Redo 重放完整前后值（kinematics 整值恢复前清空旧字段，避免遗漏 omitempty 的关联），一次撤销恢复转换前约束、位姿和映射。转换成功退出预览并切回装配设计，显示最新正式模型。

边界：最多 16 个机构、各 64 个驱动/研究/干涉分析、1024 条发布映射；每机构 32 个单元、64 关节；owning Product 64 单元；研究 2..500 帧、100..120000 ms 预算、最长 3600 s；DMU 2..128 检查单元、pair×frame≤8192、每 RPC 不同 B-Rep 总计≤256 MiB；输入/结果 JSON 分别≤8/16 MiB。它们是资源门禁，未证明工业容量或实时频率。没有动力学、多驱、可动嵌套机构、稀疏后端、碰撞连续性、碰撞自动回避、开放曲面穿透、按 pair 配置豁免或复杂规律。
