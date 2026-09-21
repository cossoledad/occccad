# Sketch 投影支线

> 状态：待实施。现有基线是同 Part 的 Edge/Vertex 正交投影、更新、Reconnect 与 Detach。返回[统一路线](README.md)。

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

## 3. PROJECTION-BOUNDARY：Face Boundary Projection

目标：在活动草图中选择平面或曲面 Face，一次创建其外环/孔环的关联二维边界集合。

范围：

- 新增 typed `ADD_EXTERNAL_PROJECTION_GROUP`，selection capability 明确允许 Face；
- 服务端从权威 B-Rep/TopologyManifest 枚举 Face boundary Edge，并为每个成员绑定 PersistentSelection；
- Worker 复用 Point/Line/Circle 投影，并补齐 Arc、Ellipse 与受控 BSpline snapshot；
- 结构树以一个 projection group 管理成员，支持整组/单成员显示、Reconnect 和 Detach；
- Profile Builder 仍只消费明确闭合且无歧义的成员集合；seam、退化 Edge、重复投影和孔环方向产生稳定诊断；
- 覆盖上游长度变化、孔增加、Edge split/merge/delete、Undo/Redo、冷重建和正式 Router。

验收：选择 Pad 顶面后一次得到可辨识的外环和孔环；正常上游编辑保持成员 identity；成员拓扑变化不按数组位置错绑。

### PROJECTION-ARC：单 Edge 的部分圆弧合同

在 Face group 之前先扩展现有单 Edge 投影，使布尔交线等 trimmed circular Edge 可以形成 `ARC` snapshot：

- naming evidence 增加圆所在平面的稳定 X/Y 基向量、规范起始方向和有向 sweep，不能只用圆心、轴、长度及 OCCT 参数区间猜测端点；
- Worker/Proto/Go/Web 同步增加 `ARC` snapshot、START/END/CENTER 子元素和 fixed geometry 约束适配；
- 对 seam、反向参数化、跨 `0/2π`、近似整圆、镜像 frame 和投影后退化建立 corpus；
- 单 Edge Arc 与随后 Face Boundary group 使用同一曲线表示和诊断，不增加只服务某个布尔案例的旁路。

在该子批次完成前，部分圆弧必须稳定返回 `EXTERNAL_PROJECTION_TYPE_UNSUPPORTED`；新建/重连命令原子失败并保留旧 Head，不能静默创建 `UNRESOLVED_EXTERNAL` 或提交错误 artifact provenance。

## 4. PROJECTION-SECTION：Section 与 Silhouette

目标：提供显式的“与草图平面求交”和“沿指定方向投影轮廓”命令。

范围：

- `SECTION` 输入 Face/Body + 活动 Sketch support，输出零到多条精确相交曲线；
- `SILHOUETTE` 保存显式投影方向/坐标框架，不读取相机；
- 为多分支、闭合/开放、周期 seam、切触退化和容差定义 solution identity 与诊断；
- 增加 branch 选择、更新后的保留/失效、Reconnect/Detach 和资源预算；
- 二维 snapshot 支持 Line/Circle/Arc/Ellipse/BSpline，并保持 3D witness 与 2D 曲线偏差报告。

PROJECTION-SECTION 依赖 PROJECTION-BOUNDARY 的 projection group、成员身份和 UI，不阻塞 Publication 或 FEATURE-REVOLVE-HISTORY Revolve naming。

## 5. 与 Surface/3D Wire 的边界

PROJECTION-BOUNDARY/PROJECTION-SECTION 的输出属于某个 Sketch 的二维 ExternalGeometry，只服务于草图约束与 Profile。Surface/3D Wire 的 `ProjectCurve`、`IntersectionCurve`、Boundary 和 CurveOnSurface 是独立的三维 WireFeature/SurfaceFeature，必须保存 3D curve、pcurve/UV、支撑面和多解 branch。不能把二维 Sketch snapshot 提升为权威三维曲线，也不能让 Surface/3D Wire 反向复用浏览器显示折线。

## 6. 执行依赖

PROJECTION-ARC → PROJECTION-BOUNDARY → PROJECTION-SECTION。它们复用现有 Part/naming 基线，可独立于装配主线推进；不依赖三维曲面模块。先统一单 Edge ARC snapshot，再复用到 Face group，避免为一个布尔案例添加旁路。三维曲线与曲面能力见[候选方向](candidates.md)。

## 7. 能力边界与缺陷处理规则

投影问题先按领域合同分类，再决定实现：

1. 已声明支持的输入未产生合同结果，属于缺陷，修复必须贯通命令原子性、Revision 状态、artifact provenance、诊断和回归 corpus；
2. 尚缺稳定表示、identity/evidence 或下游约束语义的输入，保持明确 `UNSUPPORTED` 并进入对应路线图批次，不以局部坐标猜测、浏览器折线或 OCCT local ID 临时补齐；
3. 创建/重连失败不产生新 Head；已连接依赖在上游更新后失效，才形成可检查的 FAILED Revision；任何 Revision 都不能引用与自身模型 Manifest 不一致的制品；
4. 当前未发布数据直接以唯一新语义重建，不为已污染的开发 Revision 增加读取 adapter、双写或兼容分支。建立发布基线后再按版本化迁移规则处理旧数据。

每个缺陷修复都应证明它恢复了现有合同，或把缺失能力登记到明确批次；不得为了单个复现案例扩大持久 schema 或绕过整体 evaluator 顺序。
