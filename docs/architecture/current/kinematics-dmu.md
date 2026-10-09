# 小机构与基础 DMU

> 返回[当前架构目录](../../CURRENT_ARCHITECTURE.md)。这里描述代码合同；复现、演示及尚未验收项见[实施与验证记录](../../../plans/kinematics-dmu.md)。更丰富的关节、仿真和碰撞流程仍在[目标合同](../target/kinematics-dmu.md)。

## 定义、实例与历史

Product 的 `kinematics` 保存 Mechanism 和 MotionStudy，运行结果独立保存为制品。Mechanism 保存稳定 ID、直接子实例 ID 和 Joint；Joint 保存两端实例局部坐标系（毫米原点、x/y/z/w 单位四元数）、零位、正方向和有单位的限位。运动坐标以第一端 Z 为正轴，Revolute 的相对 X 方向决定角度，Prismatic 使用第二端原点沿第一端 Z 的有向位移：`q = (raw - zero) / direction`，direction 为 ±1。Ground 固定该实例研究基准姿态。局部坐标系是显式定义值，不从瞬时 DOF、零空间、网格索引或邻近几何推导关节。

一个 Mechanism 属于一个 Product；其直接子 Part（包括多 Body Part）和子 Product 都是刚体运动单元。子 Product 内部位姿冻结，未加入机构的 owning Product 直接子实例固定在基准姿态。当前 UI 在根 Product 打开机构面板；定义实例局部原点和四元数，尚无拓扑拾取生成关节或 Engineering Connection 映射。

`SAVE_KINEMATICS` 适配到版本化领域命令 `occccad://product/kinematics/set`，只修改 `product.kinematics` PropertySlot。提交必须提供当前 `versionId`，并继续使用已有事务幂等、Head/sequence CAS、最终 ChangeSet、Outbox、Undo/Redo。保存定义及其历史重放不执行装配位置编辑，不改原约束或正式位姿；引用缺失、单位或坐标系不合法则拒绝。定义合法不等于该研究可运动，权威方程及 DOF 门禁在运行时检查。

## 完整硬方程与帧判据

复用生产装配输入冻结/编译器和数值内核；机构编译只把局部 frame 转为同一内核的 Point/Axis/Plane descriptor。所有活动、未停用、非 Measured 装配定义必须有编译方程，缺失时拒绝运行。已捕获的 Fix Together 关系展开为硬刚性方程；捕获未完成或请求要修改 Relative Fix 基准时拒绝。

| 关节 | 硬方程 |
|---|---|
| Ground | 基准世界 Pose 的 FIX |
| Rigid | 原点重合、Z 同向、X 同向 |
| Revolute | 原点重合、Z 同向；自由坐标为绕 Z 的连续相对角 |
| Prismatic | Z 同轴、X 同向；自由坐标为沿 Z 的有向位移 |
| 单坐标驱动 | 有参考 Z 轴的 ANGLE，或点至平面的 ALONG_SECOND_NORMAL DISTANCE |

保留所有原装配方程及全部闭环关节，不树化删除闭环。MotionStudy 可明确指定 `replaceDriverConstraintId`；只有同端点、同有向局部几何坐标的独立 ANGLE/DISTANCE 方程能被本次运行的硬驱动替换。组约束、不对应的坐标和未知 ID 均拒绝；替换只作用于冻结方程，不停用或修改 Product 约束。编译记录 retained/replaced constraint IDs，驱动不是 DragTarget 或软偏好。

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

`POST /api/documents/{id}/motion-runs` 校验 viewer 权限及 baseRevisionId，冻结 schema=1、policy=grounded-single-coordinate-v1 的完整 view、关节/规律/方程、solver profile 和 exact geometry refs；冻结前后检查同一个 Product Head。内容 digest 绑定输入，MOTION_SNAPSHOT 制品进入现有 MOTION_STUDY Job；actor/document/requestId 幂等，同 requestId 不同输入返回 409。Job 执行只读冻结输入；中途 Head 变化不混入新版本，也不推进任何 Head。

运行结果为 MOTION_RUN 制品；Job 的 SUCCEEDED 意味着结果已保存，研究自身 completed/status 和各帧结论决定实际质量。研究停止、数值失败、限位和部分 DMU 都可保存为可查看结果。FinishMotion 原子核对 RUNNING、lease owner、attempt 和未过期 lease；取消请求决定 CANCELED 终态，保存已完成帧，迟到 attempt 不覆盖结果。已有取消监视器取消 RPC，完成制品保存使用有界 WithoutCancel 上下文。排队即取消尚无帧；进程崩溃后未持久化的内存帧不能保证恢复，租约重领重新从冻结基准运行。

`GET /api/jobs/{id}/motion-run` 校验任务访问和 Product viewer 权限，并检查结果制品大小、种类和 SHA；历史版本结果可只读获取。已终止研究不在同一 Job 上手动 Retry，重新运行生成新 Job，保留旧结果；保存之前的基础设施自动重试及租约重领继续沿用现有队列。当前列表沿用用户最近任务列表，最多 100 项，无结果分页索引或独立共享研究库。

Web 的定义保存走 Domain Command；任务列表由 TanStack Query 获取，草稿、播放、定位为暂态交互。面板关闭、上下文/Revision 变化清理播放/高亮并请求取消当前任务，代际门禁忽略迟到响应。旧结果只能显式加载并显示冻结版本标识。回放使用冻结 view 和整帧所有 owning unit Pose；视口拒绝不完整加载的帧，立即批量设置、不逐零件插值。播放中限制编辑，退出或“恢复正式姿态”恢复当前正式 view，不逐帧写 Revision，不提供仿真姿态写回。结果行按完整实例 + Body 高亮，问题帧按钮跳至已保留的干涉/未完成帧。

边界：最多 16 个机构、64 个研究；每机构 32 个单元、64 关节；owning Product 64 单元；研究 2..500 帧、100..120000 ms 预算、最长 3600 s；DMU 2..128 检查单元、pair×frame≤8192、每 RPC 不同 B-Rep 总计≤256 MiB；输入/结果 JSON 分别≤8/16 MiB。它们是资源门禁，未证明工业容量或实时频率。没有动力学、多驱、可动嵌套机构、稀疏后端、碰撞连续性、碰撞自动回避、开放曲面穿透、按 pair 配置豁免或复杂规律。
