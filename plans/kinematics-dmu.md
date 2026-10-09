# 小机构与基础 DMU：实施、验证与人工验收

当前合同见[机构与 DMU](../docs/architecture/current/kinematics-dmu.md)，长期扩展见[目标合同](../docs/architecture/target/kinematics-dmu.md)。本轮已打通定义/单关节、闭环硬驱动及回放、静态精确 DMU、离散采样检查四个环节。浏览器实机验收由维护者完成，以下结果没有把 Node 或后端测试视为浏览器验收。

## 代码接线

- `services/internal/workspace/kinematics_model.go`、`kinematics_compile.go`：独立定义、正式命令/PropertySlot、稳定实例局部 frame、保留完整装配方程及明确同坐标驱动替换。
- `motion_snapshot.go`、`motion_run.go`、`motion_demo.go`：冻结版本与 exact refs、数值内核硬帧门禁、连续分支/限位/有界恢复、正式命令四杆演示。
- `services/internal/api/motion.go`、`cmd/occccad-jobs/motion.go`、`internal/jobs/service.go`：权限/请求幂等、Artifact/Job、lease/attempt 条件完成、取消后的部分结果。
- Worker Proto、`internal/geometry/dmu.go`、Artifact staging、GeometryPool、`kernel/occt/src/dmu.cpp`：多 B-Rep 通过原有 Router 路径做 OCCT 实体分类和距离。
- `motion-study-panel.tsx`、CommandRegistry 目录及 Workbench/Viewport：定义、运行、已保存结果、整帧播放/暂停/定位/单步/复位、scope/间隙、结果高亮/问题帧；不改变主题。

PostgreSQL/SQLite 未发布 baseline 均增加 MOTION_STUDY job type。开发库如保留旧 baseline checksum，须按项目授权使用统一 `invoke data.reset --yes` 后启动；普通迁移不会静默改写旧库。

## 实际验证（2026-10-09）

构建使用当前 Conan/OCCT 7.9.1、CMake Release，未修改三维求解器算法或精度。验证只运行改动影响范围：

| 层次 | 实际证据 |
|---|---|
| OCCT CTest | 2 个 DMU 场景：分离、接触、穿透、包含、重合、间隙、源形体不变、曲面无效和取消 |
| 真 Worker 运动学 | Revolute、Prismatic、四杆 9 帧硬驱动/闭合；Rigid；零位与负方向；60°→780°连续转角；限位停止；取消/预算保留已完成帧；输入/执行/非有限诊断失败分类及可保存结果；自由度不支持；保留原角度约束与显式同坐标替换；并发运行不修改冻结输入，相关 Go race 检查通过 |
| 权限/历史/任务 | 定义 PropertySlot、正式 Undo/Redo 和位姿不变；stale Save；请求幂等/碰撞；未授权拒绝；历史结果权限；错误 attempt、过期租约/重领后旧 owner 拒绝、取消结果、迟到重复完成及禁止覆盖旧结果的手动 Retry |
| Router/Artifact/Job 完整集成 | 正式命令创建真实四杆，冻结后改变 Head，再执行并保存 21 帧；每帧 4 个独立实例、6 个 pair 的 OCCT DMU，运动学合格而干涉结论保留；取消后保存 2 帧；当前姿态 DMU |
| 嵌套/多 Body/scope | 一个刚体子 Product 含旋转/平移的双 Body Part；5 个检查单元，各帧两后代 Body 距离 4.5 mm 保持不变；完整 path+Body 独立，scope 只检查指定一对；错误后代 Revision 拒绝；全部范围不能略过缺精确几何单元；scratch 无泄漏 |
| 真实制品后端 | 完整集成分别经过测试远端制品 staging 与当前配置 S3；都使用正式 GeometryPool Router 和真实 Worker，而非 RPC stub |
| Web | 相关 Node 场景覆盖整帧选择、冻结身份、缺帧/缺组拒绝、实际 Three.js 后代刚体变换、立即批量应用、非法四元数拒绝、单位转换、活动任务及命令注册；TypeScript/生产构建通过 |

复现（仓库根目录，先完成 Worker 构建，再启动 Worker 测试；测试期间不要重新链接同一 Worker）：

```sh
cmake --build build/cmake/release --target occccad_geometry_worker occcad_geometry_scenarios -j2
ctest --test-dir build/cmake/release -R 'geometry/DMU' --output-on-failure
OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/build/cmake/release/workers/geometry/occccad_geometry_worker" go -C services test ./internal/geometry ./internal/workspace ./internal/jobs ./internal/api ./internal/workbenchconfig ./internal/control ./internal/database -run '^(TestMotion|TestDMU|TestArtifactStagingNestedReferencesAndCleanup|TestSQLiteMigrationsReopenAndConstraints|TestMigrationCatalogValidation|TestGeometryPoolRoutesSolveAssembly|TestAssemblyHistory|Test.*Catalog)' -count=1
OCCCCAD_TEST_MOTION_S3=1 OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/build/cmake/release/workers/geometry/occccad_geometry_worker" go -C services test ./cmd/occccad-jobs -run '^TestMotionStudyPersistedRouterArtifactJobLifecycle$' -count=1
OCCCCAD_TEST_GEOMETRY_WORKER="$PWD/build/cmake/release/workers/geometry/occccad_geometry_worker" go -C services test -race ./internal/workspace -run '^TestMotionConcurrentFrozenRunsIndependent$' -count=1
pnpm --dir web/apps/cad test -- motion-study activity-projection command-registration workbench-registration-lifecycle transform-transition
pnpm --dir web/apps/cad build
invoke context-audit
git diff --check
```

S3 专项读取当前开发配置，只为测试制品使用现有桶；fresh SQLite 测试不重置应用数据库。完成 S3 验证后，按根 AGENTS 的长期授权执行 `OCCCCAD_ENV_FILE="$PWD/.env" invoke data.reset --yes`，清理当前应用 SQLite `/home/ganjb/project/occccad/services/data/occccad-local.db` 全部表/迁移、专用 S3 `occccad` 桶全部对象及版本/删除标记/未完成分片、`services/data` 制品/暂存，并重新迁移成功；桶保留，未处理其他数据库或目录。执行时 API/Jobs/Router/Worker 没有占用。应用现为空开发数据，可直接创建演示。未运行无关全量单元测试、浏览器测试、截图或视觉验收。PostgreSQL 基线定义同步修改，但本轮数据库业务测试使用 SQLite，未声称 PostgreSQL 实机迁移/并发已验证。这是正确性验证，无性能基线/P95/工业容量/60 Hz 结论。

## 实机演示与期望结果

1. 仓库根运行 `invoke run.app --build-type=Release` 启动后端，另一个终端运行 `invoke run.web --mode=api` 启动实际 API 模式前端（Mock 明确不支持本功能）。清库后注册或重新登录，新建并打开一个空 Product，在 Assembly Design 的“组件定位”工具栏选择“机构与基础 DMU”。
2. 点击“在空 Product 创建四杆闭环演示”。它通过普通命令创建 3 个矩形 Pad Part、4 个实例和 Ground + 4 Revolute；ground/coupler 共享 40 mm Part 但保留不同实例。长度为 ground 40、crank 20、coupler 40、rocker 30 mm。初值 crank 60°；默认研究从 20° 至 120°，3 s、21 帧、0.5 mm 间隙。创建是多条正式命令，不是跨文档原子大事务；中断可能留下已创建的 Part，换一个空 Product 重试。
3. 点击“运行已保存研究”，等待 Job 结束。应保存 21 帧，运动学“合格”；这些矩形杆在铰接处有意重叠，DMU 应显示 VIOLATION 并保留穿透/间隙结果，不能期待所有 frame PASS。点击结果行检查对应完整实例与 Body 高亮，点击问题帧定位。
4. 播放、暂停、单步、时间输入/滑块定位、复位研究；观察完整闭环整帧切换。点击“恢复正式姿态”或关闭面板应回到正式装配姿态，Revision 不因播放改变。重新打开面板，在已保存运行列表重新选择结果，应可以回放。
5. 将驱动 Revolute 上限设为 75°，保存定义后重新运行；应在逐帧/细分限位门禁停止，保留之前合格采样帧并提示 LIMIT_REACHED。“取消运行”也应保留已完成帧；排队阶段即取消无可回放帧。
6. 恢复正式姿态，在“当前装配姿态 DMU”选择两 Body 的 scope、指定间隙并检查；静态结果明确“运动学未检查”。可清空 scope 检查全部。模型中添加空 Part/缺精确几何时全部范围应拒绝，选择明确可检查的 scope 后才可继续。
7. 保存新的定义/修改 Product 后，显式加载旧运行；应提示旧版本，并只读显示冻结模型。关闭/退出/切换标签及 Revision 更新不应留下播放覆盖；迟到运行或加载结果不能抢占当前显示。

## 剩余验收与已知限制

- 待人工验证真实 WebGL 的冻结几何加载、整帧显示、高亮、工具禁用、退出恢复、标签/Revision 切换及取消/迟到响应。当前后端/Node 证据不覆盖这些实机行为。
- 当前 local frame 用数字定义；不支持拓扑选择自动生成关节、不自动映射 Engineering Connections、不支持可动嵌套子机构。刚体内后代几何随所属刚体运动，允许任意形式的 owning Product 直接 Part/多 Body Part/子 Product。
- 局部 rank/DOF 与分支门禁不证明全局可运动；奇异位形或未知硬约束见证可能停止，报告不收敛而非无解。基准固定，禁止软拖动代替驱动；未进行通用收敛专项或大型测试集。
- DMU 仅有效实体、指定 pair 和离散采样；开放曲面无效/未完成会 INCONCLUSIVE。无 CCD、动力学、仿真姿态写回、自动避碰、复杂 laws、多驱、按 pair 忽略/接触豁免。铰接处正常几何重叠也会报告干涉，首版不静默过滤。
- 每采样帧重新物化/加载所需 B-Rep，隔离输入和结果优先；未新增跨帧几何缓存或新碰撞后端。保存结果有大小预算；进程崩溃前未持久化的帧不能保证保留。查看结果沿用当前用户最多 100 项 Jobs 列表。
