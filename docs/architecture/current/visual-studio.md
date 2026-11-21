# 曲面、光照与预览表现

当前显示策略以曲面法线保留曲率，以柔和照明表达形体，并适度减少显示三角形。结构树继续覆盖于视区，不占用额外画布；几何、外观和观看偏好的所有权不变。具体测量与图片生成见[验证方法](../../validation/README.md#工作台与显示)。

| 范围 | 源码入口 |
|---|---|
| 显示精度与曲面法线 | [kernel.hpp](../../../kernel/api/include/occccad/kernel/kernel.hpp)、[surface_normals.cpp](../../../kernel/occt/src/surface_normals.cpp)、`OcctKernel::tessellate` |
| 统一 Worker 输出 | `workers/geometry/src/main.cpp::fill_evaluation`；Go `geometry.Client` 不覆盖未指定的精度 |
| 材质、环境与阴影 | Web `visual-tokens.ts`、`cad-material-factory.ts`、`studio-environment.ts`、`scene-presentation.ts` |
| 缩略图 | `services/internal/visual/glb.go`、`thumbnail/{scene,raster,color_space,natural_edges}.go` |
| 结构树 | `specification-tree.tsx`、`tree-layout.ts`、`workbench.css` |

## 从精确曲面到显示

```text
不可变参数 / 冻结 BREP → Worker / OcctKernel::tessellate
 → 隔离 Shape 副本剖分 → 同一 Face UV 节点的曲面法线
 → TessellationResult → make_glb 的 NORMAL 与 edgeProfile
 → 服务端制品登记 / Naming 关联
 → VisualRepository → makeGeometry → MeshStandardMaterial
 └→ Go visual.Decode → 缩略图 prepareMesh → 光栅化 / PNG
```

应用的默认均衡档采用 **0.15 mm** 线性偏差与 **0.30 rad** 角偏差。Worker 的未指定精度请求采用此档。精细档沿用原来的 0.05 mm 与 0.18 rad，高精细档采用 0.02 mm 与 0.10 rad。精度由[全局选项](object-scene-properties.md#场景与显示规则)选择并作用于派生显示。较粗剖分影响近看轮廓及显示拾取近似程度。屏幕误差 LOD 属于后续设计。

普通节点法线为 `unit(∂S/∂u × ∂S/∂v)`，经 Face 的 Location 转到 Part 局部坐标，并按 REVERSED 朝向翻转。使用 OCCT `GeomLib::NormEstim` 的 D1 / D2 估计；奇异极点的 D2 方向符号再由同一 Face 的有向三角形核对，防止小尺度球极点反向。该核对仅决定符号，方向仍来自曲面。实现依据与当前安装的 OCCT 7.9.1 [GeomLib 源码](https://github.com/Open-Cascade-SAS/OCCT/blob/V7_9_1/src/GeomLib/GeomLib.cxx)对应。

UV、曲面或可用法线缺失时，保留有效曲面样本，其余用同一 Face 内的面积加权三角法线降级；不跨 CAD 面光顺。法线求值分批检查取消。正常路径不再计算一遍三角平均法线。`normals_ms` 独立计时，`surface_normals` 日志区分求值数和降级数；两者不与 `faces_ms` 重复相加。实例旋转与缩放仍由渲染器 normal matrix 处理，不在相机变化时重算法线。

标准 GLB `NORMAL` 已能承载结果，没有新增领域 Proto 或持久身份。Worker 快照键包含实际精度及离散策略；工作区 evaluator `part-solid-generators-v78-studio-balanced-surfaces` 区分法线算法，新平面剖分另由显示策略限定输入键，见[离散快照](geometry-representations.md#拓扑感知的离散快照)。旧 Revision 不被读取操作重写，既有 GLB 的法线也不被标记为新算法结果；通过正式重新求值获得新制品。

## 默认观看配方

| 参数 | 默认 | 目的 |
|---|---|---|
| 背景上下端点 | `#e1e6ec` / `#f4f6f8` | 与文档卡片、空预览共用冷中性色 |
| 新建实体 / 继承实体色 | `#8e9faf` | 在浅背景上保留厚度；显式对象色优先 |
| 实体粗糙度 / 金属度 | `0.50 / 0.02` | 哑光低金属感，保持宽缓反射 |
| 工程曲面粗糙度 | `0.38` | 浅弧面可见柔和亮带；显式粗糙度优先 |
| 场景环境反射强度 | `reflection × brightness`，默认 `1` | 从场景环境取光的标准材质使用该系数，反射由有限面积反射卡与粗糙度共同决定 |
| 主光方位 / 仰角 | `−155° / 32°` | 增加方向层次，减少浅曲面的均匀受光 |
| 半球 / 主光 / 补光 / 轮廓光 | `0.28 / 1.35 / 0.18 / 0.14` | Z-up 半球、偏主光的连续照明 |
| 阴影 | `2048² PCF`、半径 `3`、强度 `0.28` | 沿用尺度适配偏移和稳定接收面 |

环境由四张有限面积的程序反射卡组成。视口启动时烘焙一次 256 尺寸的 PMREM，再按当前光源基准采样，销毁时释放。不同法线和观察方向产生连续反射变化，便于观察浅曲率。标准材质的粗糙度与金属度语义见 [MeshStandardMaterial](https://threejs.org/docs/pages/MeshStandardMaterial.html)。

光源基准默认「固定于场景」。视角变化时入射方向保持世界坐标，PBR 的镜面反射仍随观察方向变化。选择「跟随视角」后，半球光、主光、补光、轮廓光和已有环境纹理共同使用 `R = Qcamera × inverse(Qiso)`，`Qiso` 取标准 ISO 视角。主光方向为 `R × d(azimuth,elevation)`，方向光目标仍取可见模型范围中心。环境通过 [Scene.environmentRotation](https://threejs.org/docs/pages/Scene.html#environmentRotation) 设置采样 `R⁻¹`，旋转时继续使用同一张 PMREM。平移、缩放和投影切换保留照明基准。两种模式下背景、地面、模型和世界网格都使用各自的坐标。Onshape 的[建模视口帮助](https://cad.onshape.com/help/Content/View/camera_and_render_options.htm)列出了着色、边线和视图控制。这里的光源与阴影关系由本项目定义。

新建 Part 保存创建时的材质快照；已有模型的已保存外观、导入色、分面色和自定义观看设置保持优先。未指定外观的对象继承当前系统角色配色。第 14 版偏好只升级完全匹配旧创建配方或旧工程光照方向的字段。第 16 版为旧场景偏好补齐固定光源基准。

## 阴影缓存与时间稳定性

关闭 renderer 与主灯的 shadow autoUpdate，只有深度输入变化时请求更新。每次实际渲染通过现有 Mesh 索引检查可见投影物体的几何和属性缓冲版本、drawRange、世界矩阵、材质显隐、透明度、面向、alphaTest、裁剪和图层。光源方向、阴影相机的朝上方向、模型范围、地面高度和阴影参数进入定义键。Three.js 的渲染控制见 [shadowMap 更新机制](https://threejs.org/docs/pages/WebGLRenderer.html)。

实例动画、可见性和几何变动会重取包围盒，原位修改位置缓冲时先更新几何边界。「固定于场景」在纯相机运动时复用阴影图。「跟随视角」在实际渲染的旋转帧更新同一张主光阴影图，平移、缩放及静止复用它。近地平线时接收面阴影在主光仰角 5° 至 15° 之间渐变，低于范围时隐藏地面阴影，零件之间仍可投影。关闭阴影或编辑草图时跳过阴影图绘制。WebGL context restored 会再次标脏，释放视口时移除监听。相机旋转时基于已缓存的模型范围更新光源方向和接收面范围，接收面继续位于固定的世界高度。

缓存不移动地面，不改变近远裁剪或自然边线。地面、透明阴影和 AO 的既有边界见[场景属性](object-scene-properties.md)。当前没有跨实例表面 draw 合批；occurrence 保留私有表面包装、面组和 BVH，`SurfaceGeometryPool` 共享可复用的 position / normal / index BufferAttribute。属性共享不等于一次 draw，也不表示视口全部辅助属性只有一份。

## 缩略图的颜色与计算

服务端优先使用 GLB 的有向曲面法线。完整有效时跳过三角法线哈希表；缺失时保持同 Face 降级。每实例预投影顶点并计算一次顶点照明，在不透明 / 透明两遍中复用。颜色为近似照明：`E = 0.38 + 0.72 max(n·l,0) + 0.16 max(n·f,0) + 0.08 max(n·h,0)^10`，双面曲面按观察侧处理。它不是实时 PBR 的复制，不模拟 PMREM、物理粗糙度响应或地面阴影。

表面着色改为 `sRGB(linear(baseColor) × E)`，避免直接将非线性 RGB 字节乘光照造成过暗；线性工作空间的依据见 [Three.js 颜色管理](https://threejs.org/manual/pages/color-management.html)。输入通道只解码一次，输出用 4097 项查找表，无逐像素幂运算；给定调色板和照明范围的单通道量化误差由测试限制在 1 byte 内。

Go 解码已识别的 `edgeProfile` 并验证数组、有限值与局部边/面关联，未知来源忽略，缺失时保留拓扑线。缩略图使用 5° / 35° 权重，光顺强度为 0.12，锐边强度为 0.88。光顺线加深已经受光的底色，验证共面或接缝后才抑制。颜色过渡使用预计算表，透明排序按其近似光栅合同。交互视图区的混合与遮挡规则见[自然边线](natural-edges.md)。

renderer 升为 `png-v12-studio-normals`，沿已有预览身份按需排入任务，新旧 PNG 隔离。640×400、2 倍超采样、取消 / 超时、资源预算及 PNG 编码策略保持；预算计入新增 profile 节点。空场景回退与真实 PNG 使用同一浅色背景，不绘制虚构零件。

## 结构树

行高统一为 30 CSS px，虚拟化步长、命中区域和连接线使用同一常量。文字、辅助线、工作主体和琥珀色选择层次更明确，背景用静态渐变薄层提高可读性；不使用背景模糊，不改变画布尺寸，空白区域仍允许视区交互。上下文菜单项仅在打开对应行时构建；Escape 关闭菜单，语义身份、多选和正式命令入口保持。

树仍加载完整节点索引。大树的首次加载和交互提交时间需在真实视口分别测量。

## 深度缓存与对象索引

`SurfaceDepthPass` 缓存最近一次完整成功的深度结果，仍在每次 invalidate 中绘制正常颜色帧。深度可写成 `D = Rasterize(P × V × T × position, index, groups, visibility, clipping)`；实际依赖还包括相机层、near/far、画布尺寸、清屏色／透明度、材质显隐／透明度、绘制顺序和几何缓冲引用／版本。检查对象、矩阵、材质和面组，不扫描顶点、三角形或拓扑边。颜色、粗糙度、灯光与 AO 强度不改变这些深度输入，相应颜色／uniform 仍正常更新。

只有普通不透明单对象且不需要 AO 或多对象深度裁决时才跳过预通道。透明场景复用全表面深度和必要的不透明深度两张目标；AO 与不透明场景的线性视深度颜色附件沿原规则工作。代理继承 Mesh 的图层和 renderOrder。剖切系数变化更新结果，只有剖切启停改变编译条件时才将深度材质标为需要重编译。

缓存不依赖 UI 事件手工标脏：每次从实际可见 Mesh 核对输入，实例运动、缩放、显隐和几何替换均会失效。绘制前将缓存置为无效，两次必要通道都完成后才接受；异常后恢复调用者 render target／viewport／scissor，后续帧重新尝试。切换 renderer 或 context restored 事件也失效，销毁时释放目标、记录和事件监听。`setCaching(false)` 仅用于对照诊断，没有增加用户选项。

`SceneRenderIndex` 通过 Three.js 的 childadded／childremoved 事件维护 Mesh、Points 和已登记屏幕更新对象的集合。初次附着、子树增删、重挂和释放时维护成员，属性仍从真实对象读取，祖先显隐仍沿真实 parent 链核对。阴影准备和深度收集按 Mesh 集合检查，屏幕线更新只处理登记对象，晚注册的父标记在登记时移到已有子标记之前，保持同帧祖先依赖；点尺寸只处理 Points；不再为这些动作每帧递归整棵场景树。屏幕更新回调仍执行，Three.js 矩阵更新／渲染递归和小辅助子树的裁剪范围计算仍保留。

索引与数值快照分别占用 `O(objects)` 和 `O(meshes + material groups)` 的 CPU 工作集，没有复制共享几何或增加 GPU 目标数量。完整性依赖使用正常 Object3D 增删接口，私自替换 children 数组不是支持的生命周期；同父内排序不改变成员。该管线面向现有静态 CAD Mesh，检测到骨骼、InstancedMesh 或 morph 时不复用静态快照，不代表已交付这些几何的深度代理支持。缓存只减少相机／模型不变时的预通道，导航或运动仍需重画深度。验证条件及相同像素对照见[验证方法](../../validation/README.md#工作台与显示)。

## 后续显示优化候选

按需 invalidate、阴影缓存、深度缓存、对象索引、大 GLB 有界 Worker 解码和实例表面属性共享均已有实现。剩余候选尚未交付：

| 当前证据与入口 | 候选及验证边界 |
|---|---|
| 静态相机 32 实例缓存命中后仍有 66 次总 draw | 按同几何、材质和面组合批，或研究 InstancedMesh；必须保留 occurrence 选择、面色、透明排序、剖切和显隐，不把实例序号作为持久身份 |
| `surface-geometry.ts` 对私有包装按需构建 indirect BVH；大文件解码已有 Worker | 用真实多实例测量首次拾取与 retained heap，再研究加速结构共享／后台构建，保持 triangle／面组映射正确 |
| DPR 上限为 2，AO／深度按 drawing buffer 尺寸工作 | 确认填充率瓶颈后评估导航期间自适应分辨率或显示 LOD；停止导航恢复质量，截图质量和精确选取身份保持 |
| 缓存仍逐帧核对可见 Mesh／面组，虚线回调仍可投影其线段 | 用真实大装配区分输入检查、屏幕更新、渲染提交和 GPU 成本，再决定是否引入更细的显示版本／更新策略，不能凭小样本削弱失效完整性 |

定向验证使用软件渲染、少量实例；未覆盖原生 GPU、大型工业装配、长时间运行或完整 WebGL context loss／环境重建。context restored 的缓存失效有逻辑验证，不能将其称为完整图形上下文恢复验收。
