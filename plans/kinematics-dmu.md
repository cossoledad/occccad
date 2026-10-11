# 基础运动机构实施与验证

> 基础设计见[DMU 完整设计](dmu-mechanism-redesign.md)，最新入口与体验见[用户流程设计](dmu-ui-workflow.md)，当前合同见[Current Architecture](../docs/architecture/current/kinematics-dmu.md)。机构位置、产品位置与两套约束继续独立；入口和空机构清理按最新流程执行。

后续的接合对话框和单对话框仿真整改见[用户流程设计](dmu-ui-workflow.md)，该设计取代运行设置与回放分离、播放控制放在工具栏的旧呈现。

## 本次实施

- 服务端增加只读 application 准备与版本化 ACTIVATE/EXIT 命令，按目标方程从当前 accepted 求解；无产品关系退出持久保留位置。机构仅保存成员 accepted，空切换不生成 Revision/Undo，仍记录幂等调用。
- 前端使用完整 ApplicationDisplay 原子移交姿态与关系；资源释放不恢复产品位置，连续请求保留上次显示，取消回命令基准，保存先采用权威结果。URL 恢复所选机构；关闭回放等待合格帧采用。
- Applications 改为机械装置/模拟分组；接合、命令、固定零件、运动规律为机构下分类，模拟下投影同一源机构。真实子约束共享产品名称格式、图标、选择与所属 Joint 编辑，显式 owner/role，持久编号参与历史。
- MotionLaw 独立，Simulation 引用 Law；物理 DOF 与 Command 后 DOF 分开，Command 覆盖采用真实坐标方程求解。入口编辑明确选中的机构，否则直接新建；退出清理本会话未操作的空机构。
- 轴向偏移与平面方向分别设置，方向进入正式定位关系；固定件与多实例化共用三维／树输入到所属实例的投影。移动组件通过正式 Session/token 保存机构位姿和求解状态，保留 Product 真相与一次 Undo/Redo。
- 显式转换支持 accepted 及 Run 两种来源，无仿真也能转换；独立产品 Constraint/Connection/Quantity 身份，只有 ADD/一致 REUSE/CONFLICT，没有隐式 UPDATE。目标删除后创建新身份，完整目标方程、CAS 与一次补偿保留。

## 操作与复现

启动最新构建：`invoke run.app --build-type=Release`、`invoke run.web --mode=api`。命令目录嵌入 API，必须重建/重启后生效。开发历史数据不加兼容；需要重置时按根 AGENTS 使用统一入口。

1. 用正常 Part 草图/拉伸创建轴和转子，插入 Product，先把转子移到 `(22,15,7)` mm。产品可没有装配约束。
2. 未选机构时点击“机构”直接新建；先在树中选中机构再点击则编辑该机构。未确认定义或移动便退出，本会话创建的空机构自动删除；已有机构保留。
3. 固定件预选轴所属的任意元素，或进入后从视图区／树拾取整个实例；旋转接合选择两轴及两定位平面，设置偏移和平面方向。查看真实“重合.n (A,B) / 偏移.n (A,B)”子约束与固定件下 Fix；双击关系聚焦对应接合字段。
4. 连续改偏移，确认、取消及刷新均以机构 accepted 为基准。退出按产品关系重新计算：无关系时当前位置保留、重开仍保持；有关系时可以回到产品关系规定的位置。
5. 接合勾选角度命令或显式创建 Command，创建运动规律，再创建引用该规律的“仿真定义”；“运行仿真”打开同一个对话框，在其中计算、播放、暂停、定位或停止后台计算。每个运行挂在对应模拟下，双击指定结果回放；没有结果选择下拉框或“已保存结果”工具栏入口。关闭当前版本结果先采用合格帧，再回到机构编辑。
6. 在当前 accepted 用工具栏“转换为产品约束”，或从仿真对话框的合格 Run 帧转换，生成计划并一次提交，默认保留旋转 DOF=1；明确锁角才增加角关系。取消帧转换返回同一回放和同一帧。重复一致映射复用，定义改变报告冲突，不自动同步。一次 Undo/Redo 恢复/重放关系、位姿及映射。
7. 机构编辑时用“移动组件”从视图区选部件并拖动操纵器，产品约束不参与本次操纵；最终候选核验机构自由度与驱动资格，保存成员位姿及求解状态。退出后产品自身约束重新生效。装配中的多实例化同样接受三维预选或进入后的实例拾取。

## 验证记录

2026-10-11 体验细化：

- Workspace 领域检查最终通过（18.9 s），覆盖 workspace/modelcore；相关 `TestAssemblyInteraction|TestMechanism|TestKinematic|TestMotion` 通过。新增机构候选场景验证 Product 约束与位姿隔离、非成员归属及补偿历史。并行浏览器执行期间，既有 100 ms 预算场景曾在首帧完成前耗尽；停止浏览器后串行复验通过，未扩大预算或改变断言。
- Web 聚焦场景通过（11/107），覆盖整实例选择投影、结果树 Job 身份、支撑选择、目录、命令注册和多实例化；最终 TypeScript/生产构建通过。API/目录相关 Go 检查通过，API 与 Jobs 已重建并重启。
- 标准浏览器 9 项用例分批通过：新增／已选机构入口及未操作清理；四支撑与清除；两结果节点的独立回放；关闭／取消后台任务；迟到响应门禁；标准重命名／删除；目录及 accepted 转换；真实关系所属字段编辑；创建失败保留活动机构。更新了旧用例默认固定首个零件的假设，最后两项复验通过（4.2 min）。Mock 只验证交互，不提供数值证据。
- 真实圆柱面／定位面用例通过（3.0 min）：正式 API 建模、Publication 支撑、视图区固定件拾取；UI 设置 9 mm 和相对方向，两条正式关系均为 OPPOSITE；Native Worker 完成两次 5 帧仿真，关闭采用所选帧。两个结果节点各对应独立 Job，双击指定结果不再选择运行，关闭后继续编辑。首次计算因旧 Jobs 构建缺少方向字段导致快照校验失败，同步重建后通过。
- 新增真实操纵用例通过（1.6 min）：多实例化从视图区拾取源实例；Product 存在 SPACE Fix 时，机构仍可从视图区选部件并拖动，保存机构成员位姿和求解状态，Product 位姿与约束保持一致。一次 Undo 恢复成员归属，一次 Redo 恢复移动；退出时 Product Fix 恢复产品位置，机构保留。修正了缺少机构求解状态及提交期间实时刷新会触发额外补算历史的问题。
- 当前截图 `build/acceptance/dmu-revolute-dialog.png`、`dmu-simulation-dialog.png`、`dmu-mechanism-move.png` 均已查看。验收使用现有开发配置和正式 API/Native Worker，没有清空开发数据库或 S3；本轮聚焦上述两项真实浏览器场景，未重跑其他真实用例或全仓测试。
- 完整 Web 仍被既有 `m4.snapshot.current-render-bindings` 回归锁阻断；`tests/assembly-contract` 与 HEAD 无差异。日志：`build/agent-logs/20261011-015940-424012908-web-scenarios.log`。保留该基线，相关场景及最终构建单独验证。

2026-10-11 对话框与用户流程整改（本轮体验细化前的历史验证）：

- `go -C services test ./internal/workbenchconfig -count=1`、`go -C services test ./internal/api -run Toolbar -count=1` 通过，覆盖唯一目录、API 投影和播放命令退役；API 已重建并重启加载新目录。
- `pnpm test -- motion mechanism-selection command-registration toolbar` 通过（8/105 个匹配场景），最终生产构建通过。清除轴解除该端平面、替换后只选择缺失字段、Publication 名称按 occurrence 查找、全局命令与局部播放的边界均有覆盖。
- 标准浏览器 6 项相关用例分批通过：圆柱面/定位面 Publication 拾取及清除重选；单对话框回放、重新计算、关闭保存失败保留原帧及重试、帧转换取消返回原回放；关闭保留后台 Job、显式停止计算；迟到响应门禁；基础目录无播放/复位等命令，accepted 转换计划显示机构已确认位置而非空目标或旧回放；真实子关系聚焦所属字段。最后三项复验 4.4 min，accepted 计划及目录用例追加复验通过（1.1 min）。Mock 仅作为交互合同，不是几何或数值见证。
- 新增真实浏览器用例通过（3.6 min，PostgreSQL + S3 + Release Worker + 正式 API + Vite WebSocket + 软件 WebGL）：用正式草图/拉伸创建两零件，发布真实圆柱面及 z=0 定位面；机构、固定件、旋转接合及角度命令通过 UI 创建，形成两条真实机构约束，保留 Publication 和 PersistentSelection；规律/仿真定义也通过 UI 保存。单个对话框中完成 5 帧仿真、播放/暂停、起点/终点及单步，然后重新计算生成第二个独立运行；关闭权威保存所选帧四元数，继续打开旋转接合编辑。此用例验证树中的真实曲面 Publication，不声称已逐项验收所有直接鼠标命中或其他关节。
- 实际界面截图：`build/acceptance/dmu-revolute-dialog.png`、`build/acceptance/dmu-simulation-dialog.png`，均已查看。最终 context-audit、diff 检查通过。
- 完整 Web 再次受既有 `m4.snapshot.current-render-bindings` 合同锁阻塞；`tests/assembly-contract` 与 HEAD 无差异。日志：build/agent-logs/20261010-234402-198928306-web-scenarios.log。未修改该基线，本轮未重跑无关服务端全量几何测试，也未清空开发数据。

2026-10-10 基础机构专项（历史验证记录）：

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
