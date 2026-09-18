# P8 后 Sketch 投影能力扩展设计

状态：计划；P11J 可在 P8 完成后独立启动  
基线：P8 已实现同一 Part 内 Edge/Vertex 的正交 ExternalGeometry、更新、失效、Reconnect 与 Detach  
路线图入口：[associative-design-roadmap-after-p7.md](associative-design-roadmap-after-p7.md)

## 1. 为什么选择 Face 不能直接复用当前 Project

Face 不是二维 Sketch Geometry。一次 Face 选择至少可能表达三种不同意图：

1. **Boundary**：把 Face 的外环和内环边界投影到活动草图；
2. **Intersection**：计算 Face/Body 与草图支撑平面的精确交线；
3. **Silhouette**：沿显式方向计算轮廓线。

三者的输出数量、分支、更新规则和失败条件不同，不能用一个 `PROJECT_FACE` 后由 Worker 猜测。浏览器当前相机方向也不能成为持久建模输入。命令必须保存明确 mode、稳定来源、方向/支撑、成员策略和 branch evidence。

## 2. 统一领域合同

在现有单项 `ExternalGeometry` 之上增加投影结果组，而不是把 Face 伪装为一条曲线：

```text
ExternalProjectionGroup
  group_id                 stable ID
  source                   Face PersistentSelection
  mode                     FACE_BOUNDARY | SECTION | SILHOUETTE
  membership_policy        CAPTURED | LIVE
  direction?               explicit local/world direction
  branch_evidence?         selected solution IDs and witnesses
  members[]                stable ExternalId references
  source/result digest
  status + diagnostics

ExternalGeometry
  external_id              stable member ID
  projection_group_id?
  source                    Edge/Vertex selection or derived solution identity
  2D authoritative snapshot
```

- `group_id`、`external_id` 和 derived `solution_id` 是身份；Face wire 序号、OCCT local ID、mesh index 和数组位置不是身份；
- 第一阶段 Face Boundary 使用 `CAPTURED` membership：创建时明确绑定每个边界 Edge。split/merge 不会静默改变成员数量，而是通过 history 解析、歧义或失效诊断处理；
- `LIVE` membership 只有在稳定 member matching、tombstone 和下游约束迁移规则完成后开放；
- 多曲线结果保留全部稳定 solution，用户选择的 branch 进入定义。禁止“每次取最长曲线”；
- 更新顺序继续固定为 naming resolve → projection/section → Sketch solve → downstream Feature；失败时清除过期 snapshot，不消费旧几何。

## 3. P11J：Face Boundary Projection

目标：在活动草图中选择平面或曲面 Face，一次创建其外环/孔环的关联二维边界集合。

范围：

- 新增 typed `ADD_EXTERNAL_PROJECTION_GROUP`，selection capability 明确允许 Face；
- 服务端从权威 B-Rep/TopologyManifest 枚举 Face boundary Edge，并为每个成员绑定 PersistentSelection；
- Worker 复用 Point/Line/Circle 投影，并补齐 Arc、Ellipse 与受控 BSpline snapshot；
- 结构树以一个 projection group 管理成员，支持整组/单成员显示、Reconnect 和 Detach；
- Profile Builder 仍只消费明确闭合且无歧义的成员集合；seam、退化 Edge、重复投影和孔环方向产生稳定诊断；
- 覆盖上游长度变化、孔增加、Edge split/merge/delete、Undo/Redo、冷重建和正式 Router。

验收：选择 Pad 顶面后一次得到可辨识的外环和孔环；正常上游编辑保持成员 identity；成员拓扑变化不按数组位置错绑。

## 4. P11K：Section 与 Silhouette

目标：提供显式的“与草图平面求交”和“沿指定方向投影轮廓”命令。

范围：

- `SECTION` 输入 Face/Body + 活动 Sketch support，输出零到多条精确相交曲线；
- `SILHOUETTE` 保存显式投影方向/坐标框架，不读取相机；
- 为多分支、闭合/开放、周期 seam、切触退化和容差定义 solution identity 与诊断；
- 增加 branch 选择、更新后的保留/失效、Reconnect/Detach 和资源预算；
- 二维 snapshot 支持 Line/Circle/Arc/Ellipse/BSpline，并保持 3D witness 与 2D 曲线偏差报告。

P11K 依赖 P11J 的 projection group、成员身份和 UI，不阻塞 P9 Publication 或 P11A Revolve naming。

## 5. 与 P16 Surface/3D Wire 的边界

P11J/P11K 的输出属于某个 Sketch 的二维 ExternalGeometry，只服务于草图约束与 Profile。P16 的 `ProjectCurve`、`IntersectionCurve`、Boundary 和 CurveOnSurface 是独立的三维 WireFeature/SurfaceFeature，必须保存 3D curve、pcurve/UV、支撑面和多解 branch。不能把二维 Sketch snapshot 提升为权威三维曲线，也不能让 P16 反向复用浏览器显示折线。

## 6. 推荐执行顺序

```text
P8 complete
  ├─ P9 Publication 主线
  ├─ P11A Revolve naming
  └─ P11J Face Boundary Projection
         └─ P11K Section / Silhouette

P16 Surface/3D Wire 在其表示和质量门禁就绪后独立进入
```

若近期优先改善当前 Sketcher 工作流，可在下一批直接领取 P11J；它不会改变 P8 已冻结的单 Edge/Vertex ExternalGeometry 合同。
