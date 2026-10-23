# 平台与领域约定

这些是后续开发须保持的语义边界，不是当前功能或部署能力清单。当前实现由[主题目录](../current/README.md)路由；任务顺序只见 [CAD 核心主线](../../../plans/cad-core-roadmap.md)。

## 模型、命令与历史

- Document 是业务身份，Workspace 显式可变，Revision 不可变。显示名允许重复；业务 ID 与内容摘要不能混用。
- 参数定义、正式关系和模型快照是业务真相。导入模型的冻结 BREP 与身份种子是重建输入；派生 BREP、Naming、Visual、缩略图和分析结果带来源，可重建，不能反过来替代参数模型。
- UI Command、暂态 Interaction/Preview、Domain Command、Compute Job 分层。候选求值在事务外；最终以幂等请求、Transaction/ChangeSet 和 CAS 原子接纳。预览及鼠标轨迹不形成 Revision。
- Undo/Redo 通过正式补偿或重放追加历史，不抹除已有事务；失败回执未知时先恢复原请求，不另造写入意图。
- PropertySlot、参数源、依赖、摘要和补偿须贯通。表达式用受限、量纲检查的 ID-bound AST；禁止隐藏读取可变 Head、求值环或执行任意脚本。
- 当前未发布，不为实验数据维持 adapter、双写或过渡迁移；发布或外部数据保留边界后的旧 Revision 可读性及追加迁移规则见[根指南](../../../AGENTS.md#当前开发数据边界)。

## CAD 身份与求值

- 持久引用使用 typed stable ID、InstancePath、Publication、PersistentSelection。名称、数组位置、OCCT 指针/local ID、三角形与 mesh 序号均不能成为跨 Revision 身份。
- Body 是建模历史和求值单元，Solid 是几何结果；不能因材料连通性变化重新分配 Body 身份。依赖指向明确的上游阶段，不能读取自身或下游最终结果。
- Profile 来自正式连接与精确曲线；显示采样和相近坐标不能代替拓扑关系。轮廓或支撑丢失时必须显式诊断，不能静默换区、换面或退回默认平面。
- Naming 只采用真实生成/修改/删除历史与有版本的语义邻接闭包；缺失、歧义和类型变化明确失败，不按最近几何或第一个候选修补身份。
- Quantity、坐标系、长度/角度单位、容差与分支意图显式。协议边界转换 SI 与 mm/rad；显示舍入不进入几何、摘要或正式选择。
- 缓存身份覆盖规范输入、依赖快照、完整来源及 evaluator/kernel/solver/policy 版本。缓存缺失后从冻结输入恢复，不保证不同实现逐位相同。
- 几何有效性、拓扑覆盖、材料、Naming 和显示门分别检查；内核 IsDone 不等于可接纳。失败显示可引用成功 Visual，但不能作为当前成功几何参与拾取、导出或验收。

## 产品、装配与证据

- 定义与 occurrence 分离；共享 Part 定义不共享位姿与端点身份。完整 InstancePath、来源 Revision、坐标变换和逐层权限都须保留。
- FOLLOW_HEAD 的接受与 PINNED 的冻结显式。跨文档写入沿正式设计事务/更新计划检查权限与各自 CAS；不从界面开关推导原子性。
- Product 中的关联设计沿 Part ContextInput、Product ContextBinding 和派生 ContextVariant；普通共享定义编辑不新增并行模式。
- 装配硬可行、运动偏好收敛、rank/DOF、分支连续性与冲突证明分别报告。预算/取消、局部非收敛、输入和执行失败不能被解释为全局无解。
- 机构定义及位姿独立于 Product 约束；转换显式验证并提交。瞬时 DOF 不等于有限关节或行程；插值显示帧不能成为求解或提交见证。
- 分析结果绑定输入 Revision、依赖几何和算法策略；过期或证据不足须明确显示。接口、测试和截图存在均不能证明完整用户闭环。

## 计算、制品与安全

- 粗粒度远程、细粒度本地；逻辑领域不自动成为网络服务。Sketch solver 保持独立接口并与 Part Worker 同进程；进程拆分需有负载、隔离或许可证依据。
- Worker 消费冻结纯值与 ArtifactReference，不持有业务数据库真相，不向领域 Proto 泄漏第三方类型或操作系统路径。正式调用须贯通 client、Router、Worker、制品和最终 CAS。
- Job 至少一次投递，按效果幂等；attempt、lease、deadline、cancel 和迟到结果门禁显式。对象校验及发布先后清楚，上传成功不代表业务提交成功；孤儿对象不反向改历史。
- 文件、表达式、插件和 Worker 输入不可信，需约束大小、深度、时间、内存、网络、权限及副作用。引入服务、重大依赖或许可证变化先澄清；不从旧设计候选推断已选型。
- 服务指标、kernel、RPC、命令和浏览器耗时分别测量；包含子阶段不能相加。定向/Mock、真实 Worker、浏览器、数据库后端及容量证据各自说明范围。

具体开发、验证与文档维护规则见[Agent 工作流](../../development/agent-workflow.md)和[文档维护](../../development/documentation.md)。
