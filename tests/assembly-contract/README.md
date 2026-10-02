# 六类装配约束可执行合同

这里是六类约束的跨实现执行入口，不是第二套持久 Constraint 模型。唯一生产目录在 [services/internal/assemblycontract/catalog.json](../../services/internal/assemblycontract/catalog.json)；本目录 `catalog.json` 是指向它的相对 symlink，不维护另一份矩阵。数学/产品预期见[目标合同](../../docs/architecture/current/assembly-constraints.md)，当前设施与执行记录见[当前 Product](../../docs/architecture/current/product-assembly.md)，剩余开发见[装配计划](../../plans/README.md)。

## 文件与证据职责

- `catalog.json`：`schemaVersion=1`、`contractVersion=assembly-six-families-v2`，六族、子类型、unary/binary/group 角色、八类 descriptor/稳定派生支持、参数单位/依赖、交换/方向/branch、模式/激活、秩/退化与分阶段失败合同。Point–Curve 明确为 Line/Circle，Point–Surface 为 Plane/Cylinder/Sphere/所选叶 Cone；Contact 11 个解析分支与一般秩已冻结，不声明任意曲线/曲面。规模随新增专用 case 变化，以 validate 输出和派生报告为准，不在 README 永久锁定数量。
- `runner.py`：选择并校验同一目录；C++ 映射到现有 GTest，Go 调用真实包内函数/既有测试，TypeScript 读取目录调用现有规则；集成走正式 Router/Worker。纯模型与带 scalar test double 的编排测试各有 `evidenceKind`，不冒充几何或数据库集成。
- `baseline.json`：目标/案例摘要、实现声明下限及测试断言源摘要的回归锁，不另维护覆盖表。目录新增可以扩展；删除目标、改预期、降声明、改已有断言会失败。C++/Go 的 `fixture.assertionHelperSymbol` 必须指向实际调用的断言 helper，其源码纳入同一锁，不能只锁空包装 TEST 而遗漏共享几何/生命周期断言。真实合同演进或合法测试调整必须连同目标依据与锁的变更一起审查，不用自动 bless 绕过失败。
- `test_runner.py`：目录/执行器的完整性与负例检查，不计为产品能力验证。

实现字段与执行 verdict 独立。`implemented` 仅指该层的窄组合；`partial/missing/unknown` 不可提升为完整产品能力。`specific-combination` 与 `representative-shared-foundation` 明确区分，复用生命周期或入口测试不代表每种几何组合都完成。`caseIds` 是可追溯映射，fixture 保留在原模块，不复制 solver/command 逻辑。

六个默认必需层为 `ui/domain/resolution/workerSolver/lifecycle/historyReplay`。即使当前公共 v2 声明这些层 implemented，也必须实际执行该 capability 对应的专用断言才能验收；生产能力不依赖 PASS 数量，源码/UI、模型编排、真实数值、Router/Worker/数据库历史仍各自标记证据层。

报告根据 `requiredLayers`（默认六层）和适用的 specific-combination case/`requiredCaseIds` 分别推导 `targetStatus` 实现覆盖、`verificationStatus` 执行完整性与 `acceptanceStatus`。缺具体层测试、NOT_RUN、ENVIRONMENT_BLOCKED、真实失败都不成为完整验收；拒绝未实现项和共享基础测试不能替代具体组合证据。实现补齐可提升覆盖，不再由 profile 名称永久固定 PARTIAL。集成测试按实际 native selector 分别运行，避免一个历史测试失败把另一真实通过 fixture 误标失败。

## 执行

从仓库根目录执行，依赖沿用既有 Debug CMake、Go、pnpm/TypeScript 和 Python 3 标准库：

```sh
python -m unittest discover -s tests/assembly-contract -p 'test_*.py'
python tests/assembly-contract/runner.py validate
python tests/assembly-contract/runner.py baseline --output build/assembly-contract-baseline
python tests/assembly-contract/runner.py gaps --output build/assembly-contract-gaps
python tests/assembly-contract/runner.py composition --output build/assembly-contract-composition
```

`baseline` 只执行目录中已纳入既有验证基线的 case；尚未纳入的目标断言显示 NOT_RUN。`gaps` 执行全部选中目标断言，实际失败返回 1；未来未实现项目继续保留为 TARGET_NOT_IMPLEMENTED，缺少目标测试显示 MISSING_TEST，不改成 PASS。两种检查都要求实际执行且成功的断言；空选择、拼错 ID、过时映射或零测试不能返回绿色结果。未执行、skip 或缺环境独立记录；选中的必需基线 case 未运行返回 2。可选集成环境阻塞不使数学/模型基线失效，但报告始终保留阻塞，不能称作集成通过。

`composition` 是更严格的收口门：全部选中 capability 的 `acceptanceStatus` 必须为 ACCEPTED，所有必需实现层和具体证据均齐全且实际通过。缺测、NOT_RUN、ENVIRONMENT_BLOCKED、缺 fixtures 或集成 skip 都不能返回绿色 COMPOSITION。筛选后的成功只证明该选择范围；完整验收必须运行无 capability/family/layer/adapters 缩范围的完整入口。baseline 通过不等于完整六族通过。

定向选择可组合 capability、family、layer 和 case；未知值与交集为空报错：

```sh
python tests/assembly-contract/runner.py baseline --family Angle
python tests/assembly-contract/runner.py baseline --capability offset.point-axis
python tests/assembly-contract/runner.py baseline --layer domain
python tests/assembly-contract/runner.py baseline --case composition.finite-joints
python tests/assembly-contract/runner.py gaps --case offset.plane-plane.first-normal-editor
python tests/assembly-contract/runner.py baseline --family Offset --output build/offset-after-baseline
python tests/assembly-contract/runner.py gaps --family Offset --output build/offset-after-gaps
```

`--adapters` 可限定实际执行设施；排除必需 case 仍报告 NOT_RUN，不获得完整基线通过。`--build-type Release` 使用既有对应构建目录，执行器不会下载依赖或启动新平台。C++ 每次先构建 scenario target；Go 使用 `-count=1` 及结构化事件，子测试跳过不能被父 PASS 隐藏；Web 复用 `pnpm test` 的场景发现与零匹配失败规则。新增 Web 场景同时调用目录/锁校验，因此 `pnpm test -- assembly` 会发现目录映射退化；Go/C++ 新断言也进入原模块的测试入口，没有修改共享构建系统或 `invoke check` 路由。

要运行真实 Router，先完成统一 Worker 和解析 fixture 构建，生成实际 B-Rep/STEP，再执行目录；不能在集成运行期间重新链接同一 Worker：

```sh
cmake --build build/cmake/debug --target occccad_geometry_worker occcad_geometry_scenarios --parallel 2
export OCCCCAD_ASSEMBLY_FIXTURE_DIR="$PWD/build/constraint-composition/analytic-fixtures"
build/cmake/debug/kernel/occt/tests/occcad_geometry_scenarios \
  --gtest_filter=AssemblyExactSupport.ExportAnalyticRouterFixtures
export OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/build/cmake/debug/workers/geometry/occccad_geometry_worker"
python tests/assembly-contract/runner.py baseline --case integration.router-worker
```

数据库/历史/Release case 另要求明确配置 `OCCCCAD_TEST_DATABASE_URL`。既有集成 fixture 要求可丢弃数据库；不自动指向应用开发库，不静默切换 Mock，不清库。具体要求见[开发环境](../../docs/development-environment.md)。


## 按能力与风险定位

| 证据层 | 入口与主要断言 |
|---|---|
| 原生数学 | kernel/assembly/tests：composition/contact/offset/motion 场景，独立最终几何、Jacobian oracle、一般/特殊秩、子空间、冗余、不可行及有限运动。 |
| 连续轨迹 | interaction_scenarios.cpp：偏心抓取、自由纯平移、倾斜平面、必要旋转、组绕心、累计目标、往返/采样/尺度、预算恢复；日志是派生输出。 |
| Go 领域/历史 | services/internal/workspace：Quantity/稳定 AST、模式/激活、支持/组、候选/CAS/幂等、Manifest 输入隔离和版本 digest。 |
| 真实链路 | services/internal/control 的 composition、drag_trace、snapshot、edge_offset integration_test：正式 Router/Worker、真实几何与专用数据库、Preview→Commit、冷读、历史/Replay/Release；缺环境报告阻塞。 |
| Web/Three.js | assembly-engineering、motion-presentation、assembly-interaction、assembly-drag-trace、assembly-occurrence-snapshot、manipulator-snap、constraint-display/编辑候选：生产共享函数、确定时序、对象集合与帧，不冒充浏览器验收。 |

定向执行：

```sh
invoke check --scope assembly --match 'Interaction|Motion'
cd services
go test ./internal/workspace -run 'AssemblySolveManifest|AssemblyInteraction' -count=1
go test ./internal/control -run 'Assembly.*(Trajectory|Snapshot|Composition)' -count=1 -v
cd ../web/apps/cad
pnpm test -- assembly-engineering motion-presentation assembly-interaction assembly-drag-trace
pnpm typecheck
```

集成显式配置匹配源码 Worker、可丢弃专用数据库及解析 fixture。构建后再运行，不能并发重链接在用 Worker；不 reset/清理应用资源，缺环境不静默改 Mock。

## 剩余运动

engineering 读取指定 Revision 命令结果，不是完整网络刷新或局部冲突搜索。manifest 区分静态相对锚点与交互物理参考，方向是 owning Product 坐标、长度毫米。未 solved、affected scope 未覆盖或无当前版本结果保持未知。数值锚点不是 ground，gauge 不复制到每成员求和。

motion-presentation 场景检查固联整体可动、方向子空间等价、偏置轴、显示副本/浮点噪声与完整行；标记是临时只读对象，不进入拾取/持久模型。0/1/100/500/1000 行 fixture 验证投影/分页，不认证同规模求解。

## 报告与性能边界

报告派生自目录和实际结果，保存 commit/dirty、schema/version、命令/环境/选择、预期/观察、证据层与未执行/缺测/失败/阻塞。PASS 数量从报告生成，不永久手写多份。测试存在、历史运行、本次执行及维护者反馈分别标记。

性能复用 invoke performance-baseline 和固定原生/Session 轨迹。记录硬件、Debug/Release/build、活动量与连通范围、样本和 P50/P95。Debug 导数 oracle 不是生产成本；kernel/RPC 延迟不是浏览器输入至显示延迟。100 ms/60 Hz 为待测目标，工业容量和全局非凸最优未获证明。

## 人工检查

1. 自由件偏心纯平移、倾斜平面滑动、必要耦合旋转及组绕心；累计长拖、回拉、反向不漂移。
2. 删除/抑制后不重选立即拖动；取消/无变化零提交，有变化一次，Undo/Redo 全组件一致；旧 Session 仍拒绝。
3. Budget 保留未保存预览，同目标重试/取消及未知回执恢复，不保存旧目标。
4. 剩余运动查看固定/自由/轴/平面及未接地固联；缺证据明确待计算。方向标记只对应选中主体，偏置轴不错误穿过原点，关闭/切换/新版本清理。
5. 百项列表搜索/分页/定位不串目标，未知/嫌疑不变成已证冲突；技术证据按需复制。

维护者反馈当前使用场景下拖拽与约束较为稳定；剩余运动呈现仍需本次实机确认。不补造截图、测试版本或工业/性能结论。
