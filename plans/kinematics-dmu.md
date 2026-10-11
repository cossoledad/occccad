# 基础旋转机构整改与验证

本轮按维护者要求收缩范围。此前移动/刚性接合、试动、干涉应用、来源导入、替换/停用转换、自动示例已撤回，旧验收记录不作为当前功能承诺。当前合同见[架构](../docs/architecture/current/kinematics-dmu.md)。未使用旧数据兼容层，未重置开发数据或重启维护者进程。

## 实施范围

1. 查阅本机 CATIA 索引及 More About Joints and Constraints、Simulating with Laws、Recording Positions；旋转接合由普通同心/偏移约束组成，Simulation 为可编辑的独立研究对象，不是机构下轮询结果节点。
2. Joint 保存普通 AssemblyConstraint 与稳定 geometry refs/Quantity；Mechanism 保存完整合格编辑姿态。普通装配编译、预览、研究共用所选机构持久方程，不继承 Product 的正式约束，不重复生成定位方程。固定件保留持久 SPACE Fix 基准。
3. 保存前在事务外权威求解，约束/姿态同一次 Revision/CAS 提交及 Undo/Redo。预览确认/取消/重新加载不先跳回 Product 姿态；普通退出应用才恢复正式装配。关闭仿真停止冻结回放，当前合格帧通过一次保存成为机构编辑姿态，恢复编辑，不逐帧提交。
4. MotionStudy 在 Applications 下与 Mechanism 平级，保留选择/双击编辑/重命名/删除。终态结果只在对话框管理。保留真实轴/平面拾取、角驱动/线性规律、完整合格帧、取消/预算、连续角、限位、失败诊断、迟到结果门禁。
5. 增量应用复制自身持久同心/偏移并采用选中帧，默认 DOF=1；明确锁角才增加角约束。映射重复应用不增加关系，完整 Product 方程和正式求解仍验证；不替换/删除/停用旧约束，有冲突须返回普通装配处理。一次可撤销提交。
6. 删除生产示例/导入服务及机构关联干涉的冻结/执行路径，保留底层 OCCT 独立几何分析。测试夹具仅通过正式建模命令创建真实几何，未成为应用按钮或公开服务。

## 实机操作

启动最新构建：`invoke run.app --build-type=Release`、`invoke run.web --mode=api`。命令目录嵌入 API，需重建/重启才生效。已有旧机构开发数据不提供迁移兼容；需要清理时使用仓库授权的统一 `invoke data.reset --yes`，而非手工改表或另建运行数据库。

1. 普通 Part 建模：XY 圆 Ø8，拉伸 30 mm 得圆柱轴；另一 Part 的 XY 同心环外径 Ø24、内径 Ø9，拉伸 4 mm，再添加与环相交的两个矩形叶片并拉伸 4 mm。保存后插入同一 Product。
2. 使用普通装配移动命令将螺旋桨偏移，例如 `(22,15,7)` mm。首次转换用无旧约束的 Product，不需要自动示例入口。
3. DMU → 机构，检查节点立即出现、装配设计分类隐藏。固定件选择圆柱轴。旋转接合先选螺旋桨孔的轴，再选圆柱轴；两轴齐全立即预览同心。继续选择双方定位端面并设置偏移，保存。
4. 检查“旋转接合”下两个“同心 / 偏移”持久子节点、普通约束图标和高亮。双击子节点替换几何/修改偏移，保存或取消都不跳回无约束 Product 位置。退出再双击机构，保持其已接受编辑姿态；装配页仍保持自己的原始姿态。
5. 创建角驱动，再创建线性仿真 0°→360°、4 s、25 帧。MotionStudy 应与机构平级；双击可修改规律。运行后回放、单步、定位到 90°。
6. 关闭仿真对话框，等待该次保存完成；确认回到机构编辑、可以新建/编辑旋转接合和研究，姿态保持已接受帧，没有“恢复正式姿态”按钮。重新打开结果可继续定位冻结帧。
7. 在合格 90° 帧执行“应用到装配”，生成计划后提交，返回装配页检查同轴、定位及选中角度。默认仍保留旋转自由度；重开文档及普通求解认可正式关系。重复应用不增加关系，一次 Undo 恢复转换前约束/位姿。明确锁角才令 DOF=0；旧约束有冲突时不提交部分修改。
8. 验证空新机构退出清理、已有定义保留、树删除依赖清理、结果删除、任务取消及退出后迟到响应不抢占视口。本轮没有干涉功能；后续作为独立应用任务重新实现。

## 实际验证

2026-10-10 实际完成的范围：

- Go workspace/API/jobs/catalog 专项通过；覆盖普通持久方程、闭环、硬驱动、限位/连续角、取消/预算、访问控制、版本和删除。删除旧的驱动替换实现后，保留方程的回归确认其不能被静默移除。
- `TestMotionApplyGeometryFrameAtomicHistory` 使用优化构建的真实 Geometry Worker 通过。两个 Part 的几何由正式草图/拉伸/插入/移动命令生成；验证第一选择运动、持久同心/偏移和独立编辑姿态、偏移修改与 Undo、25 帧完整 360°、关闭回放采用帧后的原结果转换、默认 DOF=1/锁角 DOF=0、重复应用、完整目标约束冲突、CAS 和一次 Undo/Redo。测试不是手写动画。
- Web 逻辑场景选中 8/105 通过，含真实视口方法的姿态生命周期回归；没有运行其余无关场景。
- Chromium 浏览器专项 7 个不同场景分两批通过：进入/空节点清理、角色过滤与实时接合预览、回放/结束后编辑/转换、迟到结果、标准重命名/删除、已撤回命令与独立可编辑 Study、接合子节点双击编辑。浏览器使用 API 合同 fixture 与 SwiftShader，不作为真实后端几何拾取或数值/OCCT/CAS 证据。
- Web TypeScript/Vite 和 API/Jobs Go 构建通过；context-audit、git diff --check 通过。Vite 保留现有大 chunk 提示。

不做容量、P95 或实时帧率推断。

复现命令：

```sh
go -C services test ./internal/workspace ./internal/api ./internal/jobs ./internal/workbenchconfig -run 'Motion|Catalog|Toolbar|^TestOffset' -count=1
go -C services test ./cmd/occccad-jobs -run '^TestMotionApplyGeometryFrameAtomicHistory$' -count=1
pnpm --dir web/apps/cad test -- motion command-registration mechanism-selection toolbar
pnpm --dir web/apps/cad exec playwright test browser/motion-workbench.spec.ts -g 'dialog replay|filtered tree picks|basic mechanism command' --timeout=120000
pnpm --dir web/apps/cad exec playwright test browser/motion-workbench.spec.ts -g 'entered mechanism|late results|entered definitions|joint relation' --timeout=120000
pnpm --dir web/apps/cad build
go -C services build ./cmd/occccad-server ./cmd/occccad-jobs
invoke context-audit
git diff --check
```

未验证：真实后端浏览器几何拾取和人工视觉验收、PostgreSQL 实机回归。本轮测试的命令/Revision/Job/CAS 数据合同使用隔离 SQLite；没有新增数据库迁移或 C++/Proto 改动。未验证旧开发数据，且不承诺兼容。
