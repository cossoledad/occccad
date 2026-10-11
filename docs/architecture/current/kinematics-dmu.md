# 基础旋转机构、仿真与产品约束转换

> 当前实现合同。设计依据及验收矩阵见[完整设计](../../../plans/dmu-mechanism-redesign.md)与[对话框操作流程](../../../plans/dmu-ui-workflow.md)，验证记录见[实施记录](../../../plans/kinematics-dmu.md)，扩展候选见[目标](../target/kinematics-dmu.md)。当前仅支持根 owning Product 的固定件、旋转接合及单角度线性仿真。

## 应用与位姿所有权

“装配设计 / 机构与 DMU”是同一 owning Product 的编辑应用。进入 DMU 不创建对象；“新建机械装置”确认后才保存空 Mechanism，取消不写历史，退出不删除已确认空机构。双击机构或模拟下的机构引用激活同一稳定 Mechanism ID。应用不依赖单次 CommandDialog。

Product instance.pose 是正式产品位置，Mechanism.poses 仅保存该机构成员的 accepted 位置；两套约束独立。完整 ApplicationDisplay 合并当前环境与机构成员，临时预览和回放都是完整显示帧。机构不保存非成员历史位置，也不拥有其关系。删除接合解除关系并清理依赖，保留成员归属及当前位置；完全自由的成员也通过权威求解更新 DOF。

`POST /api/documents/{id}/mechanism-application` 只读准备切换候选，绑定当前 baseRevisionId。编辑者通过 `ACTIVATE_MECHANISM` / `EXIT_MECHANISM` 重新权威求解并 CAS 提交。进入以当前 accepted 位置为 nominal，仅启用目标机构关系；退出从源机构 accepted 位置启用全部产品活动关系。没有产品关系时保留并持久采用当前位置；有产品关系时结果可以移动。多个机构之间切换的环境按会话中有序稳定 sourceMechanismIds 合并，保存的成员仍只属于各自机构。切换失败保持原应用，空 ChangeSet 不生成 Revision 或 Undo 项，调用仍有幂等审计。

浏览器用 URL 的 application/mechanismId/mechanismSources/applicationRevision 恢复应用；刷新从已保存成员位姿开始显示，再重新验证目标机构。有序环境仅在所记录 Head 一致时恢复，且只由成功切换或定义采用更新；预览和回放不改变切换来源。有序环境引用的机构被本地删除前，先按产品方程采用当前位置。外部 Revision 变化使用当前 Product 非成员位置，清除草稿、旧回放和确认资格，再按当前 Head 验证机构。已删除的机构返回装配入口并报告上下文失效。只读用户可以准备和查看，不能提交位置或定义。

Viewport Engine 在同一次完整帧提交中替换全部 occurrence Group 的姿态及关系标记。资源清理只释放预览资源，不恢复业务位置。请求开始、拒绝或迟到不先写旧 Product 帧；取消显式回到本次命令前 accepted。保存采用服务器返回的机构基准，再清理草稿并刷新 Query。姿态与文档 Revision 匹配前保持上一显示帧。

创建保存失败时保留原活动机构及显示基准，不把尚未保存的新身份作为切换来源。定义对话框保留统一错误提示与可展开的诊断详情，取消或开始下一次请求时清除。

## 树、命名与选择

```text
Applications
├─ 机械装置
│  └─ 机械装置.1，自由度=n
│     ├─ 接合 → 旋转.1 (Part.1,Part.2) → 重合.1 / 偏移.1
│     ├─ 命令 → 命令.1
│     ├─ 固定零件 → 固定零件.1 (Part.1) → 固定.1
│     └─ 运动规律 → 规律.1
└─ 模拟
   └─ 模拟.1 → 源机械装置的引用投影
```

接合子关系是真实持久 AssemblyConstraint，包含稳定 ID、名称、Quantity、方向和 VERIFIED 等评价状态。产品与机构共用类型名称和 `名称 (实例名称,实例名称)` 格式，编号计数器与模型一起持久化并参与 Undo，不依赖当前数组下标；用户名称保留。基础阶段子关系编辑委托所属 Joint 的轴/定位字段，不提供破坏成对定义的独立删除。支持引用通过属性和编辑字段呈现，不作为默认树子节点。

树投影的 mechanismId、ownerEntityId、relationRole 明确标识所有者和角色，同一业务对象可有不同投影路径。选择、高亮、普通约束图标与标记共用装配通路，不解析显示名或约束 ID 前缀。Inspector 只读，并从当前 owning document/Revision 查找真实定义；本地命令完成后按稳定身份更新定义选择的版本，已删除对象撤销选择，几何选择不借此改写。Mechanism、Joint、Command、Law、Simulation 复用标准编辑、重命名及删除；删除清理依赖，不删除已转换产品关系。删除当前机构前先按产品方程采用位置。

## 独立的约束与权威求解

Ground 保存 SPACE Fix 的捕获基准；旋转接合恰好保存轴 Coincidence 与 SELECTED_PLANE_NORMAL_V1 定位平面 Offset。两端局部 Frame、CapturedX、零位、正方向、限位用于运动坐标。稳定轴/平面引用是定义，解析 Frame 是派生值；两定位平面不能独自确定角零位。固定基准不因 warm-start 重新捕获。

`mechanism-preview` 是带取消和 20 s 预算的只读预览。两轴齐全可预览不完整接合，平面齐全编译完整约束；全部已有机构方程保留，Product 约束完全不继承。第一选择为 MovingBody，第二选择为 ReferenceBody，采用 MOVE_FIRST_MINIMIZE_REFERENCE。前端冻结命令前 accepted nominal，连续输入只更改草稿，不逐次以预览结果重定义基准。

旋转接合对话框按“轴线重合 / 轴向定位”组织四个支撑字段，显示零件和几何来源，顺序仍是两轴、两面。轴字段通过正式 inspect 从圆柱面、圆边、圆锥面解析轴，也接受基准轴/直边；定位字段仅接受该端零件的平面面或基准平面。支撑允许替换与清除，清除轴同时清除该端定位面；替换后只推进到尚未绑定的字段。偏移、角度命令及两个生成关系直接呈现，零位/方向/限位可展开。完整输入且对应预览合格后才可保存。

`SAVE_KINEMATICS` 在事务外解析稳定支持、绑定普通约束及权威完整求解。机构、预览、运行共用 FreezeAssemblyInput，不重复从 Joint 生成另一套定位方程。成功要求 CONVERGED、运动偏好门禁、独立硬见证、完整有限单位四元数及接合闭合；客户端输入位姿仍须权威验证，不能作为合格见证。定义及成员 accepted 一次写入 product.kinematics，经 Revision/ChangeSet/CAS/Undo/Redo；失败不推进 Head。

Mechanism.evaluation 分别记录 physical relativeDOF、gaugeDOF、施加 Command 后的 drivenDOF、driverCount。Command 覆盖通过同时加入当前坐标硬方程的真实求解计算，不以简单减法推算，不保存永久角锁。树显示 drivenDOF；未计算时显示待计算。一个接合坐标不能重复创建 Command。

## 冻结运行、取消与回放

MotionDriver 只绑定 Revolute 的角坐标。MotionLaw 独立保存 start/end/durationSeconds；MotionStudy 引用 mechanismId/lawId，保存 frames/budget。服务端解析 Law 后冻结数值字段供求解器使用，拒绝缺失、跨机构引用。接合对话框可同时创建角度命令。

`motion-runs` 冻结完整 view、机构定义/accepted、所引用规律、Study、solver profile 及 policy，前后检查 Head。运行要求接地后 physical relativeDOF=1、gaugeDOF=0，加入硬角驱动后局部确定。连续角、限位、上一合格帧初值、分支、有界细分重试和硬见证沿用数值内核。失败/取消保存已完成帧及分类诊断，不把未收敛当无解。

MOTION_SNAPSHOT → MOTION_STUDY Job → MOTION_RUN 使用现有 attempt/lease/deadline/cancel 和完整性摘要。后台任务独立于面板，退出不自动取消任务。只有显式加载或当前会话前台完成的结果进入显示；迟到结果不抢占视口。终态结果删除隐藏用户列表项，保留 Job/Artifact/审计。

回放使用冻结树、几何、选择和合格完整帧，不逐零件插值、不逐帧写 Revision。关闭当前版本回放时先保持所显示帧，通过一次权威 SAVE_KINEMATICS 采用成员位置，再结束冻结显示；保存失败保留回放及错误。历史结果关闭恢复最新机构 accepted，不回写旧快照。

工具栏的“运行仿真 / 已保存结果”打开同一个“仿真与回放”对话框。仿真定义、计算进度、停止计算、结果选择、播放/暂停、起点/终点、前后单步和时间定位均在其中操作。播放按钮直接操作当前对话框会话，不进入全局 Registry；工具栏和搜索没有播放类命令。计算前若采用当前版本的回放帧，先权威保存该帧。关闭对话框切断前台完成资格，后台 Job 可继续，迟到结果不能恢复回放。打开期间冻结定义修改，关闭成功后恢复机构编辑；视图导航仍可用。

回放帧转换作为该对话框的子流程，取消转换计划返回同一回放和同一帧；成功则退出到装配设计。普通工具栏转换只从当前机构已确认位置启动。

## 显式转换为产品约束

`motion-apply-plan` 接受当前机构 accepted 的 mechanismId，或 actor 所有的冻结 Job 及合格 frameIndex；二者互斥。无需创建 Command、Law、Simulation 或 Run 也可转换。Ground 生成独立 Product Fix，Revolute 复制普通重合/偏移定义，分配独立 Constraint/Connection/Quantity 身份。默认保留旋转自由度，显式“锁定当前角度”才增加静态角约束。

映射只允许 ADD 或定义一致的 REUSE；任何定义不一致都是 CONFLICT，不执行 UPDATE、替换、停用或删除旧产品关系。映射目标已删除时重新生成新身份，不复活旧 ID。产品与机构的后续修改互不传播。

计划校验机构稳定 ID/语义、成员几何/解析版本、Run 完整性与有效帧；非成员采用当前产品位置，旧回放不覆盖后来新增/移动的非成员。显示重命名不改变 Joint 语义摘要。全部目标 Product 活动方程必须编译并验证，正式求解满足偏好和独立硬见证，且仍认可选择帧。冲突拒绝整个提交。`APPLY_MOTION_FRAME` 核对计划 digest 与 Head，一次事务提交新增约束、实际变动位姿、计数器与映射，支持一次 Undo/Redo；成功返回装配设计。

当前未实现多驱仿真、其他关节、可动嵌套 Product、干涉应用、单坐标试动、来源导入或动力学；没有旧开发数据兼容。没有容量、稳定 P95 或实时频率结论。具体已验证范围及全量检查限制见实施记录。
