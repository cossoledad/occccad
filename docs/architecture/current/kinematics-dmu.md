# 基础旋转机构、仿真与装配转换

> 当前实现合同。长期扩展见[目标](../target/kinematics-dmu.md)，验证和操作见[实施记录](../../../plans/kinematics-dmu.md)。本轮收缩此前的应用范围；静态/运动干涉应用、其他接合、试动、装配关系导入均已撤回，不是已交付功能。OCCT 独立相交/距离计算能力保留，不与本轮机构运行绑定。

## 应用与对象

同一 owning Product 的“装配设计 / 机构与 DMU”是互斥的编辑应用状态。装配页 DMU 类别中的“机构”按钮进入并立即通过正式命令创建空 Mechanism 节点；双击既有节点进入其编辑状态。本次新建、未编辑且版本未被外部推进的空机构退出时清理，已有空机构不随退出删除。应用独立于单次命令对话框；定义、仿真、转换均使用 catalog、CommandRegistry、CommandOperation 和 CommandDialog。提示/错误使用底栏和 OperationFeedback，视口没有常驻额外区域。

当前保留固定件、旋转接合、角度 Driver、线性 MotionStudy、运行、播放/暂停/单步/定位/复位、结果删除、增量应用到装配、退出。没有新建第二个机构的冗余命令、自动示例、恢复正式姿态、移动/刚性接合、试动、干涉分析或导入/替换装配关系入口。

Applications 下 Mechanism 拥有固定件、旋转接合及 Driver。MotionStudy 是独立的可编辑应用对象，通过 mechanismId/driverId 引用运动定义，与 Mechanism 平级；双击编辑其规律。它不是轮询 Job 产生的死节点。Mechanism/Joint/Driver/Study 支持已有树命令的选择、高亮、编辑、重命名和删除；删除清理依赖，不修改正式 Product 约束。运行是 Job/Artifact，仅在仿真与回放对话框呈现，不插入设计树。

此组织参考 CATIA 本机 `kinugbt0801.htm`（旋转接合的约束组合）、`kinugbt0201.htm`（按规律运动及退出保持姿态）、`kinugbt0302.htm`（独立 Simulation 可记录多个机构）。本项目把线性规律和冻结运行关联于 MotionStudy；未实现 CATIA 的多机构录制。具体页面见[交互参考](../../references/cad-workflows.md)。

## 独立且持久的约束系统

Mechanism 保存 `poses`（完整 owning Product 实例的已接受编辑姿态）。Joint 的 `constraints` 保存普通 `AssemblyConstraint` DefinitionVersion=2 定义，使用相同稳定引用、Quantity 参数、单位、方向语义和状态枚举。Ground 保存 SPACE Fix；旋转接合恰好保存 Coincidence 轴同心和 SELECTED_PLANE_NORMAL_V1 平面 Offset。树中接合只显示名称，下级“同心 / 偏移”对应实际持久约束 ID 与 VERIFIED 状态，轴/平面引用挂在对应关系下；编辑委托所属接合以保持有效的成对定义。视口直接读取这些约束，复用装配 glyph、引线、SelectionIndex 和几何高亮。

Joint 的两端局部 Frame、CapturedX、零位、正方向、限位用于运动坐标；Frame 从持久轴/平面引用解析。两定位平面不能独自定义角度零位，首次绑定捕获稳定的两端横向基准。选择复用 FeatureSelectionSession：角色过滤、精确几何解析、自动推进、已绑定字段替换；圆柱/圆/圆锥经已有 derived-role 解析为轴。引用为定义，解析 Frame 为派生值，不保存临时 solver ID。

机构的编辑、预览和研究输入从 Product 的实例/几何快照构造，但只放入所选 Mechanism 的约束；正式 Product 约束完全不继承。普通 FreezeAssemblyInput 编译持久同心/偏移/Fix 方程，机构编译器只补冻结非成员和硬驱动所需的坐标。不会又从 Joint 重复添加一套定位方程。纯数值闭环测试仍可使用内部局部 Frame fixtures；新公开定义必须有真实稳定轴/平面引用。

`POST /api/documents/{id}/mechanism-preview` 只读预览，要求当前 baseRevisionId、取消/20 s 预算及资源门禁。两轴齐全可预览未完成接合的同心方程；平面齐全使用普通完整约束编译。Ground 和全部已有接合方程始终保留。第一选择为 MovingBody，第二选择为 ReferenceBody，采用 MOVE_FIRST_MINIMIZE_REFERENCE；不临时固定参考件或放宽精度。

`SAVE_KINEMATICS` 在 CAS 事务外重新解析、绑定约束，并对变更机构执行权威完整求解；客户端姿态只作初值，不能成为合格见证。成功要求 CONVERGED、运动偏好门禁、完整独立硬见证、全实例有限单位四元数姿态及接合闭合验证。最终把约束和合格编辑姿态一起写入 `product.kinematics`，经既有 Revision/ChangeSet/CAS/Undo/Redo；失败不推进 Head。已固定实例的持久 Fix 基准不会因客户端 warm-start 被重新捕获。

机构编辑姿态与 Product 正式装配姿态是两个约束系统的状态，使用相同 occurrence Group 和 SE(3) 显示通路。确认接合、清理预览、替换字段、更新同一机构 Revision 时不先恢复无约束的 Product 姿态。取消草稿恢复机构已接受姿态；只有离开机构应用恢复最新 Product 姿态。几何/实例结构变化导致不完整姿态或失效引用时拒绝相关编辑/运行，不补猜测引用或静默丢方程。

## 冻结运行、取消与回放

MotionDriver 只引用 Revolute 的角坐标；MotionStudy 保存 `start/end/duration/frames/budget` 线性规律。所选机构接地后的 relative DOF=1、gauge DOF=0，加入角度硬驱动后要求局部确定。没有软拖拽驱动。重复约束、闭环、限位、连续角度、上一合格帧 warm-start、分支、有界细分重试和完整硬见证沿用既有数值内核；普通装配编辑门槛不降低。内部数值测试可直接冻结 Frame，公共运行只接受保存的 Study。

`POST /api/documents/{id}/motion-runs` 冻结完整 view、Mechanism 约束/编辑姿态、Study、solver profile 和 policy `revolute-owning-constraints-v3`，编译前后校验 Head。不接受 currentOnly/Analysis/DMU scope/试动请求，不加载干涉 B-Rep 或调用 AnalyzeInterference。运行结果仍以 MOTION_SNAPSHOT → MOTION_STUDY Job → MOTION_RUN 闭合，绑定 request digest、attempt/lease/deadline/cancel；迟到结果不推进业务 Head。失败/取消保留已完成帧及诊断，未收敛不是无解。

回放采用冻结 view 的树、几何、选择和整帧姿态，不逐零件插值、不逐帧写 Revision。只有显式加载或本会话前台完成的任务进入视口。关闭仿真对话框停止播放并离开冻结显示；当前版本的合格帧可作为机构编辑姿态，关闭操作通过一次 SAVE_KINEMATICS 权威校验保存，随后恢复编辑。历史结果关闭只恢复最新机构基准，不回写旧快照。继续编辑、重新运行、退出可正常操作，无“恢复正式姿态”中间状态。

后台 Job 独立于面板，退出不自动取消后台任务；用户明确取消才发送取消。退出、外部版本变化和迟到响应按原有代际/Operation 门禁清理。结果切换释放旧显示；`DELETE /api/jobs/{id}/motion-run` 删除终态结果的 user_visible 列表项，保留 Job/Attempt/Artifact 和转换审计，活动任务拒绝删除。

## 增量应用到装配

当前仅增量发布机构连接并采用有效帧。Ground 发布 Fix；旋转接合复制自身持久同心/偏移定义，重新分配稳定 Product 约束/Quantity 身份和记录映射；相同映射重复应用复用关系，避免重复。默认保留旋转自由度，显式“锁定当前角度”才增加静态角约束。规律留在 Study。

转换不导入、替换、停用或删除既有 Product 约束。完整目标 Product 方程仍全部验证，冲突明确报告并拒绝提交；用户在普通装配设计中处理后重新规划。计划核对冻结机构/几何/实例快照、结果 SHA、有效整帧、当前版本与 digest，正式装配求解须 CONVERGED、硬见证通过且仍落在所选帧。Constraint、正式 Instance pose、映射在一次 APPLY_MOTION_FRAME 的 CAS 事务提交；失败不留部分修改，一次 Undo 恢复转换前状态。成功返回装配设计并显示最新正式结果。

边界：当前只做根 owning Product 的固定件与旋转机构、单角度驱动和线性离散帧。子 Product 作为冻结内部位姿的刚体。没有移动/刚性等其他接合、干涉应用、单坐标试动、来源导入、多驱、动力学、连续碰撞或旧开发数据兼容。未证明工业容量、稳定 P95 或实时频率；实际验证范围见实施记录。
