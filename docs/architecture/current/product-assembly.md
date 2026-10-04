# Product、装配与发布

Product 模型与 Revision 是定义和引用的业务真相；求解器只消费冻结纯值。数学关系见[六类约束合同](assembly-constraints.md)，数值实现见[求解器算法](../../../kernel/assembly/SOLVER_ALGORITHMS.md)，执行方法见[装配验证](../../../tests/assembly-contract/README.md)。

## 结构与上下文

ProductInstance 保存明确引用 Revision、FOLLOW_HEAD/PINNED、局部 placement 与完整 InstancePath。共享 Part 的 occurrence 共享定义，不共享位姿和支持身份。一个多 CAD Body Part occurrence 是一个运动单元；嵌套 rigid Product 遵守 owning Product 边界，不展开为 flexible assembly。

工作台 EditSession 保存宿主 Product、唯一编辑目标、完整 occurrence、宿主/目标 Revision、工作 Body 和 activation generation。普通 Part 建模修改共享定义；ContextBinding/ContextVariant 是显式关联，不是另一编辑模式。[编辑上下文与标签](product-edit-context.md)与[建模和显示](model-display.md)拥有具体交互规则。PINNED 可查看，修改前须显式恢复跟随。普通读取/广播不打开、激活或重排标签。

Publication 用稳定目标身份，支持输出及转发；PersistentSelection 保存命名身份与解析快照，topology local ID 只作瞬时证据。Product Design Session 验证 root snapshot、typed path、目标与权限；跨 Workspace 设计事务原子检查 CAS，保存最终模型与 ChangeSet，Undo/Redo 使用正式补偿。FOLLOW_HEAD 变化由 digest-bound UpdatePlan 按叶到根接受 UPDATE_REFERENCES；失败保留合法快照与已提交子 Part，PINNED 不跟随。

参数 Publication 的关联更新计划另冻结 `parameterUpdates` 前沿（Part、源 Revision、依赖及待更新状态）。一次显式接受按参数依赖顺序预览并提交受影响 Part，最后让当前 Product 接受这些确切结果 Revision；相同 Part 的 occurrence 只重算定义，保留各自 ID 和位姿。参数源加入通知订阅图，计算不依赖页面是否打开；浏览器自动跟随已计算版本时跳过需要跨 Part 参数重算的计划，界面由“更新关联零件”确认。

接受前校验计划 digest、每个文档权限及 CAS，固定外部参数输入，不读取半途变化的 Head 作为结果。PINNED/ISOLATED 不传播，循环拒绝。失败不接纳失败候选，Product 保持原接受版本；已成功的子 Part 可保留独立 Revision，重新取得计划后恢复。该批次只更新参数外部引用，不顺带更新 Part 的其他 ContextReference。当前一次计划覆盖 Product 直接引用的 Part 及其递归参数依赖，嵌套 Product 仍沿用各自叶到根接受计划；未引入跨文档长事务。

## 定义与求值

公共六族为 Coincidence、Contact、Offset、Angle、Fix、Fix Together；Concentric/Distance/Parallel/Perpendicular/Rigid 是内部原语或明确快捷入口。唯一生产目录在 [assemblycontract](../../../services/internal/assemblycontract/README.md)，测试目录仅引用 capabilityId 并映射执行证据；生产按一次加载的只读索引查询，返回脱离缓存的副本。按钮可用性不依赖 PASS 计数，FACE/EDGE/VERTEX 不等于精确类型。

Point/Axis/Plane/Cylinder/Circle/Sphere/Cone/Frame 及稳定派生角色经 B-Rep、Datum、Publication、持久选择解析，局部到 owning Product 只变换一次。Worker 长度用毫米、角度弧度；Quantity 源值 SI 在领域编译边界转换。mesh 不是权威支持。

Literal/表达式保存 Quantity、稳定 AST 参数引用与维度。参数、方向、支持、模式原子编辑；Measured 保留驱动定义，独立 measuredValue，不加方程或移动组件，不可测时清除旧值。mode、suppressed、连接与评价状态正交，恢复保留模式并重新解析支持。

Fix Together 保存 GroupId、成员及捕获关系；先解有效内部约束，再从成功结果冻结相对关系用于组外。嵌套/重叠确定性编排，循环拒绝；失败不覆盖捕获基准。组停用不删除内部独立约束。

已接纳集合与未接纳定义分离。支持失败可保留 Broken，可解析但不可行/未收敛按命令合同保存 Impossible/NotUpdated；失败位姿不提交，不伪造 Verified、不偷偷 suppressed。Release 检查全部需要满足的活动定义，而不只检查最终子集。

## 编辑候选

Preview/Commit 共用草稿规范化，包含支持顺序、方向、参数/表达式、模式、组成员和完整上下文。求值结果/branch 属于候选证据，传输关联不作为用户意图。输入变化、迟到响应、目标切换或取消使旧候选失效。

提交保留 actor/权限、目标、Revision/CAS、定义/意图摘要、支持证据、policy/build、TTL 与交互身份校验。真实 stale 保留草稿并要求重新预览；有效候选提升不重新完整求解；成功重试先走 receipt。确认成功关闭编辑器，失败保留草稿。

合法定义完成支持解析并进入 SOLVING 后，子请求 deadline、Worker 不可用或数值未完成可产生仅定义候选（DEFINITION_ONLY）。用户显式确认才保存新定义及 NotUpdated 和结构化失败；不采用失败姿态、branch、组捕获或测量。此候选不能用于 Move，也不伪造收敛 Manifest。求解子预算为父请求保留结束时间；父取消、权限/参数错误、CAS 与数据库失败仍拒绝。新增/编辑先使旧评价失效，正常重算才能恢复 Verified。

## 连续操纵

数值 Session 冻结 actor、Workspace、owning Product/occurrence、Head/sequence、引用/定义摘要、实际已接纳活动集合、descriptor、组阶段、nominal、branch、policy/profile/build。容量 128，空闲 TTL 两分钟；不占等待用户的数据库事务。重启可使 Session 失效，不损坏持久模型。

DragTarget 是交互偏好而非 interaction-driver Fix。优先级为硬约束 → driven → held 姿态/侧向 → 剩余 nominal；静态解选择独立。pivot/frame/baseline 冻结，desired 来自累计用户输入，accepted 只作 initial guess。无 Ground 的整体运动保留。

一个在途加一个最新待发送目标，合并而不取消饥饿，pointerup flush 最终目标。可行显示帧、合格 checkpoint、warm start、最终候选分开。Budget 不等于网络超时；最多四个 continuation bridge 共用 deadline 和总迭代预算，未合格子步不运输分支或生成 Revision。

Final 同目标有界续算，未确认保留明确未保存预览，可重试/取消，不能提交旧目标。工具/命令切换先终结未提交状态；取消恢复当前权威快照，不能用旧 baseline 覆盖外部更新。SPACE 捕获不改变，显式 Move 授权的 RELATIVE 基准更新仅在最终原子命令中发生。

pointerup 后冻结最终目标、Session、文档与 generation，最终求解/提交独立于普通 UI 选择。空白点击或选择其他对象不取消、不重定向、不在完成后抢回选择。模型命令通过结束屏障等待；未提交预览或回执未知须先重试/取消，不能从临时显示位置开始下一操作。显式取消与外部失效仍有效。

成功有变化的手势一次 Move Revision，全部相关姿态/证据原子保存；中间帧、取消、无变化零提交。回执未知按 requestId 查询/幂等重试，不将本地回滚解释为业务撤销。

## 局部诊断与剩余运动

只读诊断冻结目标隔离定义及相关完整邻域，不调用会缩减集合的 admission。独立满足见证为 SAT，有限解析证书为 UNSAT，一般数值失败/预算/证书不足为 UNKNOWN。不可约须原集合 UNSAT 与必要删除 SAT 证据，不是最小基数。修复经用户选择正式 Edit/Reconnect/Suppress/Measured；版本变化使旧结论失效，正常受限拖动不触发搜索。

剩余运动列出当前 owning Product 全部运动组件。读取精确 Revision 对应命令结果，不借旧 Revision，不按 Fix/参与关系猜测。缺失、范围未覆盖或 component 未 solved 保留待计算；没有独立只读刷新入口，元数据 Revision 缺结果时保持未知。

静态证据相对数值锚点，gauge 独立；交互证据是含整体运动的物理子空间。数值锚点不是 ground；未接地固联组组内刚性、整体可动。表格、详情、按需标记共享 MotionPresentation；方向是 owning Product 坐标，偏置轴不称为世界轴，瞬时自由度不等于关节、有限行程或极限。关闭/选中变化/版本变化清理标记，不参与持久几何或约束。

剩余运动行选择自动显示有效方向；详情直接列出参考对象、平移方向、轴方向/位置和有意义的螺距，长度跟随工作台单位。显示副本以毫米尺度消除 1e-12 以下噪声，不修改原始证据或分类。analysis-guides 拥有独占几何/材质，在模型之后独立清深度覆盖绘制；箭头 64 CSS px、线宽 2 CSS px，正交/透视 zoom、resize 与 DPR 更新仅改变变换或 uniform。标记不参与模型包围盒、拾取、BVH、导出或历史。

## 显示、历史与发布

新 DocumentView 即使 GeometryKey/pose 不变，也同步 owner/version/referenceMode、InstancePath、选择与操纵绑定；开始手势从当前权威快照按稳定 occurrence 冻结，旧路径严格拒绝。约束显示按 owner/occurrence/ConstraintId 增量 reconcile，增删、模式、抑制、评价、支持与标签独立失效，请求帧并清理拾取/高亮，无 GLB 下载或完整 BVH 重建。单一手柄吸附复用可见拾取与精确查询；中心重定位不是模型移动，手势框架冻结。

SolveManifest 冻结用户定义、编译输入、descriptor/来源、occurrence、参数解析值、模式/激活、组阶段、分支/基准、policy/profile/build、失败试算与采用结果。Replay 不查询最新几何、不依赖 Session。开发数据只支持当前 DefinitionVersion=2、Quantity 和 assembly-m4m5-intent-v12/canonical JSON 摘要；旧实验定义、Offset 符号约定、pair-Rigid 持久记录和 policy 明确拒绝，不做读取修补、换号、组迁移或多代投影。Rigid 快捷创建仍生成当前正式组，内核刚性原语继续使用。已有旧开发数据可能无法读取，未自动重置。当前格式的 JSONB 数值规范化、篡改拒绝、Revision、CAS、Undo/Redo 与 Release 冻结完整保留；记录实际重放 build，不承诺跨版本逐位一致。

## 入口与限制

- services/internal/workspace：product_context、product_design_transaction、product_update_release、assembly_solve、assembly_interaction、assembly_interaction_continuation、assembly_conflict、assembly_engineering、assembly_solve_manifest。
- kernel/assembly：solver.hpp、solver.cpp 与算法文档。
- Web：assembly-interaction、motion-presentation、assembly-conflict-panel、cad-viewport-engine。
- 定向测试、报告和人工步骤见装配验证，不把测试存在或历史通过当作本次执行。

装配主体能力形成当前开发基线；已知细节暂缓，后续按实际需求修复。[暂缓问题](../../../plans/README.md#装配暂缓问题)集中记录现象、已有复现信息与边界。

维护者确认主体功能具备，当前使用场景下拖拽与约束较为稳定，仍有交互、显示细节问题；不代表工业、并发、平台或性能全部通过。dense 后端、局部分支、单次分解不可中断和非线性 UNKNOWN 仍有效；浏览器视觉与端到端/60 Hz、工业容量未完整验收。Engineering Connections 继续暂停；flexible assembly、通用最小冲突证明、稀疏后端与跨主机协作未交付。
