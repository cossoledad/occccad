# 基础运动机构实施与验证

> 最新设计见[DMU 完整设计](dmu-mechanism-redesign.md)，当前合同见[Current Architecture](../docs/architecture/current/kinematics-dmu.md)。旧实现的“进入自动创建空机构、退出恢复旧产品位置、Study 自带规律、映射关系隐式更新”已被本次整改取代。

## 本次实施

- 服务端增加只读 application 准备与版本化 ACTIVATE/EXIT 命令，按目标方程从当前 accepted 求解；无产品关系退出持久保留位置。机构仅保存成员 accepted，空切换不生成 Revision/Undo，仍记录幂等调用。
- 前端使用完整 ApplicationDisplay 原子移交姿态与关系；资源释放不恢复产品位置，连续请求保留上次显示，取消回命令基准，保存先采用权威结果。URL 恢复所选机构；关闭回放等待合格帧采用。
- Applications 改为机械装置/模拟分组；接合、命令、固定零件、运动规律为机构下分类，模拟下投影同一源机构。真实子约束共享产品名称格式、图标、选择与所属 Joint 编辑，显式 owner/role，持久编号参与历史。
- MotionLaw 独立，Simulation 引用 Law；物理 DOF 与 Command 后 DOF 分开，Command 覆盖采用真实坐标方程求解。进入应用与显式新建分开，确认的空机构不自动删除。
- 显式转换支持 accepted 及 Run 两种来源，无仿真也能转换；独立产品 Constraint/Connection/Quantity 身份，只有 ADD/一致 REUSE/CONFLICT，没有隐式 UPDATE。目标删除后创建新身份，完整目标方程、CAS 与一次补偿保留。

## 操作与复现

启动最新构建：`invoke run.app --build-type=Release`、`invoke run.web --mode=api`。命令目录嵌入 API，必须重建/重启后生效。开发历史数据不加兼容；需要重置时按根 AGENTS 使用统一入口。

1. 用正常 Part 草图/拉伸创建轴和转子，插入 Product，先把转子移到 `(22,15,7)` mm。产品可没有装配约束。
2. 点击“机构”进入 DMU，点击“新建机械装置”确认。取消未确认对话框不写历史；确认空机构退出后仍保留。
3. 固定件选择轴；旋转接合选择两轴及两定位平面，设置偏移。查看真实“重合.n (A,B) / 偏移.n (A,B)”子约束与固定件下 Fix；双击关系聚焦对应接合字段。
4. 连续改偏移，确认、取消及刷新均以机构 accepted 为基准。退出按产品关系重新计算：无关系时当前位置保留、重开仍保持；有关系时可以回到产品关系规定的位置。
5. 接合勾选角度命令或显式创建 Command，创建运动规律，再创建引用该规律的仿真；运行并整帧回放。关闭当前版本结果先采用合格帧，再回到机构编辑。
6. 在当前 accepted 或合格 Run 帧“转换为产品约束”，生成计划并一次提交，默认保留旋转 DOF=1；明确锁角才增加角关系。重复一致映射复用，定义改变报告冲突，不自动同步。一次 Undo/Redo 恢复/重放关系、位姿及映射。

## 验证记录

2026-10-10 本次专项：

- `go -C services test ./internal/workspace ./internal/workbenchconfig -count=1` 通过；命名计数器/历史、Law 引用、成员与非成员位置所有权、树投影和命令目录覆盖。
- workspace/API 的 `Motion|Mechanism|ConstraintNames|AssemblyGroup` 专项通过。
- `invoke check --scope services --match TestMotionApplyGeometryFrameAtomicHistory` 通过。真实优化构建 Worker + SQLite + 正式草图/拉伸 Product：机构约束隔离、physical 1/gauge 0/driven 0、无关系退出保留/重开/Undo/Redo、产品 Fix 在机构内忽略并在退出时生效、空目标从当前环境切换、无变更激活与幂等审计、无 Driver/Law/Study 转换；原有 360° 冻结 Job、90° 转换、复用、锁角、冲突、并发 CAS 和一次历史仍通过。新增/移动非成员不被旧 Run 覆盖，源帧 map 不被修改；目标删除后新身份、来源改变 CONFLICT；删除全部关系及命令后两个自由成员的总 DOF=12，位姿保留，退出和一次 Undo 补偿正确。
- `TestRealtimeMechanismNoopHasFrozenReceipt` 通过：正式 WebSocket 无变更调用返回锁定 Head/sequence 回执，重复调用复用；Head 后续变化时旧回执不携带新版本快照，相同请求键不同输入被拒绝。空 Product 的准备结果包含空位姿数组。无变更仍有成功审计，但没有业务事务或空 Revision。
- `invoke check --scope web --match motion` 及最终 `pnpm test -- motion command-registration tree toolbar inspector` 通过（9/105 个匹配场景）；实际 Engine 原子整帧写入序列覆盖草稿、取消、保存、退出、版本门禁及非成员保持。逻辑场景不代替浏览器或数值证据。
- `browser/motion-live.spec.ts` 首项用例通过（真实 API、PostgreSQL、S3、正式草图/拉伸、Release Worker、Vite WebSocket 代理及软件 WebGL，4.1 min）。记录 63 次主场景实际渲染，其中取消/确认/刷新/退出/重开/转换共 55 帧不出现旧 Product `(22,15,7)` 中间位置；刷新、退出、重开和转换均保留已确认 z=11。无仿真转换产生 3 个独立产品约束，Undo/Redo 正确。逐帧 JSON：build/acceptance/dmu-render-frames.json；树与只读 Inspector 截图：build/acceptance/dmu-mechanism.png。
- 同文件的真实 UI 新建用例通过（3.4 min）：只通过正常 API 准备 Part/Product，机构、Ground、Revolute 全部通过 UI 创建；树中依次拾取两轴和两定位平面，经过正式 inspect/preview/SAVE 链路，支持引用持久绑定 owning Product/occurrence。机构 accepted z=9、physical 1/gauge 0/driver 0，Product 仍为 `(22,15,7)` 且没有产品约束；退出后 Product 保留 z=9。截图：build/acceptance/dmu-created-by-ui.png。
- `browser/motion-workbench.spec.ts` 的 8 项标准交互用例分批通过：显式创建/保留空机构、树拾取推进、完整帧回放与转换、迟到响应门禁、标准重命名/删除、基础命令目录、子约束所属字段编辑，以及新建失败保留活动机构并正常退出。最终两项回归通过；错误状态持续显示并可展开诊断。首次整组执行的重命名/删除因软件 WebGL 下查询超时失败，扩大该用例的等待范围后复验通过。这些 API fixture 只验证交互，不提供数值求解证据。
- 最终 `pnpm build`、`invoke context-audit` 及 `git diff --check` 通过。

全量检查边界：

- `invoke check --scope web` 受现有 assembly-contract 冻结断言摘要失配阻塞，例如 m4.snapshot.current-render-bindings。该 fixture 源码与 HEAD 字节一致，本次未改动，未修改或自动 bless 合同锁。
- `invoke check --scope services` 全量未通过：Realtime SQLite LOCK 语法、Contact/EdgeVertex router 断言与混合 Loft Shell 等失败，workspace、jobs、catalog 等包通过。详细日志：build/agent-logs/20261010-214230-585177785-go-packages.log。未在本任务修改这些几何算法或 SQLite SQL 路由，也未将专项通过表述为全量通过。
- 专用测试 SQLite 曾因未发布迁移校验失配重建，仅处理 build/test-resources/occccad_test.db 及其 WAL/SHM；未清空应用数据库、S3 或用户制品。真实浏览器通过正常 API 创建独立验收文档。

```sh
invoke check --scope services --match TestMotionApplyGeometryFrameAtomicHistory
go -C services test ./internal/workspace ./internal/api -run 'Motion|Mechanism|ConstraintNames|AssemblyGroup' -count=1
go -C services test ./internal/workspace ./internal/workbenchconfig -count=1
invoke check --scope web --match motion
pnpm --dir web/apps/cad exec playwright test browser/motion-workbench.spec.ts
# 先启动最新 API/Worker，并加载项目 .env 登录配置
pnpm --dir web/apps/cad exec playwright test --config playwright.live.config.ts browser/motion-live.spec.ts
pnpm --dir web/apps/cad build
invoke context-audit
git diff --check
```

只对实际 fixture 与已运行路径作结论；不推断工业容量、稳定 P95 或实时帧率。未扩展其他接合、多驱、可动嵌套或干涉应用。
