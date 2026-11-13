# 自然边线

普通着色视图使用精确拓扑邻接与沿边局部法线，区分结构边和光顺连接；柔和边线加深已经受光照的底色。几何属性在 OCCT 生成快照时计算，浏览器只做矩阵变换、连续权重和光栅化。它是可重建显示证据，不改变 BREP、Naming、Revision 或持久引用。

入口：`kernel/occt/src/edge_profile.cpp` → `kernel/api/src/mesh_glb.cpp` → `web/apps/cad/src/cad/visual/` → `natural-edges.ts`、`surface-appearance.ts`、`surface-depth-pass.ts`。格式由 [Visual/Naming](visual-naming-artifacts.md)拥有，外观继承见[颜色材质](surface-appearance.md)，实际测量范围与重建方法见[验证记录](../../validation/natural-edges.md)。

## 边与夹角的定义

| 对象 | 含义与本实现的处理 |
|---|---|
| 拓扑边 | BREP 中连接顶点并被一个或多个 Face EdgeUse 使用的曲线；同一面可以沿参数接缝使用两次。边线来源于这些边，不从所有显示三角形边生成 |
| 三角形边 | 曲面离散后的内部边；通常不是 CAD 结构，不参与普通零件描边 |
| 光顺连接 | 两侧有一致方向的局部单位法线接近相同；连接仍有拓扑身份，但只做弱加深 |
| 锐边 | 两侧法线存在明显转折；本方案按连续权重逐渐过渡到结构线，不用单一角度开关 |
| 开口边界、自由线 | 分别有一个、零个 Face EdgeUse；保留结构表达。超过两个使用者视为非流形，也保守保留 |
| 共面分割 | 两个解析平面在模型容差内重合，且有一致方向；验证两侧法线后隐藏内部分割线 |
| 参数接缝 | 同一 Face 的两个 EdgeUse，且 OCCT 确认 Closed；验证法线连续后隐藏，非光顺接缝仍显示 |
| 视角轮廓 | 表面朝向从前到后变化的可见边界，可以位于 Face 内部，没有对应的拓扑边。本次没有新增全曲面轮廓追踪；外形由表面遮挡自然呈现，原有选择轮廓继续独立工作 |

令 `n₁,n₂` 为同一沿边位置、应用 Face orientation 后的局部单位法线：

```text
c = clamp(dot(n₁, n₂), -1, 1)
θ = acos(c)                       // 法线夹角，范围 0°…180°
```

不使用 `abs(dot)`，以免把反向法线误认为光顺。方向一致的共面连接和相切连接均为 `θ=0°`，直角连接为 `θ=90°`，反向法线为 `180°`。本实现不计算带符号的实体内部二面角：对于有一致外法线的凸连接，内部角为 `180°−θ`；凹连接为 `180°+θ`。因此光顺处的内部角为 `180°`，凸直角为 `90°`，凹直角为 `270°`。开口曲面通常没有唯一的实体内部角。

## 几何生成

OCCT 在精确 Shape 的深拷贝上剖分，保留每个面上每次 EdgeUse 的 polygon、UV、参数、Face orientation 和曲面 adaptor。逐个验证两侧边界节点与原 CAD Edge 折线正向或反向一致。法线来自曲面 `normalize(∂S/∂u × ∂S/∂v)`，再按 Face orientation 翻向；adaptor 已包含 BREP location。不是按重合顶点聚合法线，也不使用三角形平均法线判断精确连接。

每个原始折线段采样端点及 1/4、1/2、3/4。内部位置用 EdgeUse 的 polygon 参数和 pcurve 求 UV，两侧各自计算法线。若中间权重与端点权重线性插值相差超过 `0.02`，或采样法线变化超过 `5°`，递归二分，最多深度 5。每个原段最多生成 32 个渲染段；二进制网格缓存重复采样，端点奇异时尝试沿本段向内 `10⁻⁶` 的单侧法线。

新渲染节点插值在**原折线的弦上**；不另造曲线，不移动原来的 CAD 拾取节点，不改变已验证的三角面边界。法线对应原曲线参数位置与当前显示弦的近似关系；几何误差仍由原剖分控制。没有用整条曲线的平均夹角代替局部变化。解析平面复用恒定法线；自由线、开口、非流形和已确认的共面连接无需自适应细分。

这是有限采样启发式。采样阈值约束默认 `5°…35°` 权重，不是任意高频曲面、极端非均匀缩放或自定义角度阈值的全局误差证明。达到上限保留有限采样结果并设置 quality 标记，缺失法线则整个对应渲染段保守显示为结构线。

每段保存：两端位置、两端各自的双侧法线、原边 local locator、两侧 face locator、kind 与 quality。kind 来自实际邻接/解析平面/Closed 检查；不消费或信任导入文件的 smooth/crease 标签。浏览器也不会仅因 kind 是 seam/coplanar 就隐藏，仍需法线有效且两侧点积大于 `0.999998`。不认识的来源/版本使用原拓扑折线保守显示，已识别但损坏的 accessor/关联拒绝加载。

没有相邻面时是自由线；一个相邻面时是开口；缺 UV、pcurve、参数或可靠法线时不推测光顺。退化极点没有线，长度不超过 `10⁻¹² mm` 的弦不送入线着色器。未知旧 GLB 也不按坐标猜测邻接。原有的剖分边界不一致仍按 Visual 合同报告错误，不能用柔和线掩盖几何制品损坏。

## 数据与所有权

```text
不可变 BREP → 拷贝剖分 / 每个 Face EdgeUse
 → TessellationResult.edge_profile
 → GLB OCCCCAD_cad.edgeProfile（独立可选 schemaVersion 1）
 → Worker snapshot cache / ArtifactReference
 → 服务端添加 Visualization 与 Naming 关联 / 摘要登记
 → VisualRepository 长度 / SHA / 关联校验
 → 解码队列或 Web Worker（ArrayBuffer 转移）
 → MeshData.edgeProfile → LineSegmentsGeometry → LineMaterial
```

每段 BIN 负载 88 B：位置 24 B、法线 48 B、metadata 16 B，另有固定 JSON/accessor 开销。标准 FACE/EDGE/VERTEX primitive 保留，CAD schema 仍为 2，领域 Proto 不增加显示数组。原边 locator 只在冻结 Geometry 内定位；建模选择继续经同一 Naming 门绑定持久身份。

Worker 的快照策略键包含 `visual-edge-profile-v3` 和离散精度，缓存计数包括 profile 容量。`edge_profile` 日志记录采样子阶段、D1 次数、渲染段数和上限次数；常量平面法线的初始计算与 EdgeUse 构建计入原 `edges_ms`，不计入该 D1 次数。`edge_profile_ms` 是 `edges_ms` 的子阶段，不能再相加成总时长。

正常实线按不可变 MeshData 共享几何，法线直接用 GLB FLOAT 数组的 interleaved view，只新建 kind/quality 的小型 GPU 属性。引用计数在最后一个 occurrence 释放时 dispose GPU geometry；WeakMap 不延长 MeshData 生命周期。虚线的屏幕距离随 instance placement 变化，因此保留独立几何。原 CAD 折线保留做拾取，材质不提交绘制，避免零 alpha 的重复 draw。跨实例的表面、材质和 draw call 没有合并；这里共享缓冲不代表整个装配只有一次 draw。

独立所有权通过重新 acquire 私有几何获得，不对共享 interleaved geometry 调用 `clone()`：当前 Three.js 的该复制路径按底层 ArrayBuffer 去重，会混淆同一 GLB 中不同 typed view 的范围。实际渲染路径不使用这条复制路径，故障注入夹具也从原 profile 建立私有缓冲。

## 渲染公式与参数

着色器先以 `normalMatrix = inverseTranspose(modelViewMatrix)` 变换两侧法线，再归一化。旋转、平移、均匀缩放和共同镜像不会改变点积；非均匀缩放会正确改变实际显示夹角。退化矩阵产生无效法线时保守显示结构线。相机旋转不会触发 CPU 重新采样。

默认使用法线点积的 smoothstep：

```text
w = 1 − smoothstep(cos θsharp, cos θsmooth, c)
a = mix(asmooth, asharp, w) × coverage × objectOpacity × occlusion
```

| 参数 | 默认值 | 依据与入口 |
|---|---|---|
| θsmooth | 5°，固定 | 消化微小法线差异，使相切附近保持柔和 |
| θsharp | 35° | 足以分离圆角连接与明显转折；全局选项可调 10°…75° |
| asmooth | 0.12 | 在白、深、红、蓝实际图像上保留弱连接感；全局选项可调 0…0.4 |
| asharp | 0.88，固定 | 保留清晰结构，同时避免不透明黑色描边 |
| 线宽 | 1.35 CSS px | 沿用对象 edgeWidth 覆盖；不因模型单位和 DPR 自动变粗 |

光顺端的墨色为黑，普通 alpha 混合直接加深**已经完成表面光照的显示颜色**：

```text
Cout = (1 − a) Csurface + a Cink
w=0 且无显式边色时：Cink=0，因此 Cout=(1−a)Csurface
```

底面每个像素自身的 RGB 比例得到保留，适应分面颜色与不同受光状态；不统一盖固定灰色，不再次为线计算灯光。这里是显示空间的设计性加深，不是物理线性光能乘法。过渡区在着色器中把线性 theme 边色乘 `w`，经 Three.js 输出色彩转换后参与 framebuffer alpha 混合；锐边保留 theme 对比色，在深色表面仍可辨识。用户明确指定 edgeColor 时优先保持该颜色，但仍使用角度强度。材质透明度与已有透明遮挡衰减相乘，不伪造折射。

保持深度测试、关闭线的 depthWrite；沿用有限深度偏置 `max(16/(2²⁴−1), 0.5×fwidth(depth))`，以及既有最近表面深度通道。透明遮挡沿用减弱虚线，不把全部背面线变成透视叠加。线段以屏幕 quad 渲染，使用导数解析 coverage 和常规 alpha 混合，弱线不通过 MSAA alpha-to-coverage 量化，避免低 opacity 被截没；coverage 后只乘一次强度。Three.js 的相关约定见[官方 Material 文档](https://threejs.org/docs/pages/Material.html)。

普通边不加入朝向/灯光的额外强度变化；这样相机运动时不会跨离散视角阈值。选中/悬停恢复明确的交互颜色与线宽，精确拓扑 overlay 使用原折线，隐藏的接缝仍能高亮并完整恢复。线框及显式 EDGE 拾取显示完整 CAD 拓扑，均可辨识原本隐藏的 seam。全局“统一”选项保留人工对照入口，所有这些观看偏好不写 Revision。

## 基础、优化与边界

基础方案包含精确邻接、局部采样、连续强度、底色加深、保守降级和独立交互。随后根据实际绘制统计增加：实线几何共享、零复制法线属性、跳过透明拾取线绘制。验收用 `uniform / basic / natural` 保留三阶段投影：uniform 为恒定强度、basic 为独立缓冲的基础算法、natural 为默认优化算法。三者使用相同源 GLB 与显示条件；basic/natural 逐像素相同。

没有新增相机相关全曲面轮廓、逐帧 CPU 边遍历、跨 occurrence 批量绘制、法线量化、OIT 或边线专用后处理。现有轮廓选择算法仍基于显示网格，不能宣称精确曲面轮廓。普通缩略图尚未接入本边线 shader，仍按自身 renderer 合同绘制。

已存储且缺少 profile 的 GLB 保守显示原结构线；重建几何显示制品后获得新效果，不在读取时改写不可变 Revision，也不为此清库。极端缩放、极短密集折线的端帽重叠、贴近共面几何的深度偏置、相交透明表面排序以及粗三角网格的表面棱感仍受现有光栅管线限制。真实硬件 GPU、百万级边装配与任意曲面的采样误差上界未作验收。
