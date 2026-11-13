# 曲面、光照与预览表现

当前显示策略以曲面法线保留曲率，以柔和照明表达形体，并适度减少显示三角形。结构树继续覆盖于视区，不占用额外画布；几何、外观和观看偏好的所有权不变。具体测量与图片生成见[定向验收](../../validation/visual-studio.md)。

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

默认显示线性偏差为 **0.05 mm**，角偏差为 **0.18 rad**。唯一默认定义在内核头文件；Go 的实体、曲面、预览和交换请求不再重复填写默认值，Worker 对未指定值采用内核默认。显式请求仍可指定精度。它们约束显示离散，不改变实体建模容差、精确 BREP、Naming、单位或持久选择。较粗网格仍会影响近看轮廓和显示拾取的近似程度；法线不能补回几何轮廓，未实现屏幕误差 LOD。

普通节点法线为 `unit(∂S/∂u × ∂S/∂v)`，经 Face 的 Location 转到 Part 局部坐标，并按 REVERSED 朝向翻转。使用 OCCT `GeomLib::NormEstim` 的 D1 / D2 估计；奇异极点的 D2 方向符号再由同一 Face 的有向三角形核对，防止小尺度球极点反向。该核对仅决定符号，方向仍来自曲面。实现依据与当前安装的 OCCT 7.9.1 [GeomLib 源码](https://github.com/Open-Cascade-SAS/OCCT/blob/V7_9_1/src/GeomLib/GeomLib.cxx)对应。

UV、曲面或可用法线缺失时，保留有效曲面样本，其余用同一 Face 内的面积加权三角法线降级；不跨 CAD 面光顺。法线求值分批检查取消。正常路径不再计算一遍三角平均法线。`normals_ms` 独立计时，`surface_normals` 日志区分求值数和降级数；两者不与 `faces_ms` 重复相加。实例旋转与缩放仍由渲染器 normal matrix 处理，不在相机变化时重算法线。

标准 GLB `NORMAL` 已能承载结果，没有新增领域 Proto 或持久身份。Worker 快照策略为 `visual-surface-normals-v4`，含实际精度；工作区 evaluator `part-solid-generators-v78-studio-balanced-surfaces` 隔离新求值制品。旧 Revision 不被读取操作重写，既有 GLB 的法线也不被标记为新算法结果；通过正式重新求值获得新制品。

## 默认观看配方

| 参数 | 默认 | 目的 |
|---|---|---|
| 背景上下端点 | `#e1e6ec` / `#f4f6f8` | 与文档卡片、空预览共用冷中性色 |
| 新建实体 / 继承实体色 | `#8e9faf` | 在浅背景上保留厚度；显式对象色优先 |
| 实体粗糙度 / 金属度 | `0.50 / 0.02` | 哑光低金属感，保持宽缓反射 |
| 工程曲面粗糙度 | `0.38` | 浅弧面可见柔和亮带；显式粗糙度优先 |
| 环境强度 | `0.60` | 配合低亮背景与有限反射卡，不采用镜面默认 |
| 主光方位 / 仰角 | `−155° / 32°` | 增加方向层次，减少浅曲面的均匀受光 |
| 半球 / 主光 / 补光 / 轮廓光 | `0.28 / 1.35 / 0.18 / 0.14` | Z-up 半球、偏主光的连续照明 |
| 阴影 | `2048² PCF`、半径 `3`、强度 `0.28` | 沿用尺度适配偏移和稳定接收面 |

环境由四张有限面积的程序反射卡组成，启动时仅烘焙一次 256 尺寸 PMREM，视口销毁时释放；不下载 HDR，不捕获模型，不增加每帧环境计算。不同法线和观察方向接收到不同反射，因此亮带随真实曲率连续变化；光照固定于世界，未增加随相机跳变的分类或人工曲率染色。标准材质的粗糙度与金属度语义见 [MeshStandardMaterial](https://threejs.org/docs/pages/MeshStandardMaterial.html)。

新建 Part 保存创建时的材质快照；已有模型的已保存外观、导入色、分面色和自定义观看设置保持优先。未指定外观的对象继续继承当前系统角色配色。UI preferences 升至 14，仅将完全匹配旧创建配方和旧工程光照方向的偏好升级为新默认，保留其他自定义值。

## 阴影缓存与时间稳定性

关闭 renderer 与主灯的 shadow autoUpdate，只有深度输入变化时请求更新，使用 Three.js [shadowMap 更新机制](https://threejs.org/docs/pages/WebGLRenderer.html)。每次实际渲染检查可见投影物体的对象状态：几何与 position/index 的引用和版本、drawRange、世界矩阵、材质显隐 / 透明度 / 面向 / alphaTest / 裁剪，以及参与绘制的相机层。光源方向、模型范围、地面高度和阴影参数进入定义键。

几何、实例动画或可见性改变后重取包围盒；原位修改位置缓冲时也更新它的边界。相机旋转、投影、DPR、基础色和高亮不改变方向光深度，不重画阴影图。WebGL context restored 会再次标脏；释放视口时移除监听。关闭阴影或草图编辑不绘制阴影。每帧只检查对象及固定长度矩阵，不扫描全部顶点、三角形或拓扑边；只在几何位置变化等失效时重新计算边界。

缓存不移动地面，不改变近远裁剪或自然边线。地面、透明阴影和 AO 的既有边界见[场景属性](object-scene-properties.md)。当前没有跨实例表面 draw 合批；8 个 occurrence 仍有 8 个表面几何包装，共享解码数组和自然边线几何不等于共享全部 GPU buffer 或一次 draw。

## 缩略图的颜色与计算

服务端优先使用 GLB 的有向曲面法线。完整有效时跳过三角法线哈希表；缺失时保持同 Face 降级。每实例预投影顶点并计算一次顶点照明，在不透明 / 透明两遍中复用。颜色为近似照明：`E = 0.38 + 0.72 max(n·l,0) + 0.16 max(n·f,0) + 0.08 max(n·h,0)^10`，双面曲面按观察侧处理。它不是实时 PBR 的复制，不模拟 PMREM、物理粗糙度响应或地面阴影。

表面着色改为 `sRGB(linear(baseColor) × E)`，避免直接将非线性 RGB 字节乘光照造成过暗；线性工作空间的依据见 [Three.js 颜色管理](https://threejs.org/manual/pages/color-management.html)。输入通道只解码一次，输出用 4097 项查找表，无逐像素幂运算；给定调色板和照明范围的单通道量化误差由测试限制在 1 byte 内。

Go 解码已识别的 `edgeProfile` 并验证数组、有限值与局部边/面关联；未知来源忽略，缺失时保留拓扑线。自然边线复用 5° / 35° 权重、光顺 0.12 / 锐边 0.88 强度，光顺线加深已经受光的底色，验证共面 / seam 后才抑制。颜色过渡使用预计算表；交互 shader 的完整公式由[自然边线](natural-edges.md)拥有。缩略图没有视角相关轮廓增强，透明排序仍按其近似光栅合同。

renderer 升为 `png-v12-studio-normals`，沿已有预览身份按需排入任务，新旧 PNG 隔离。640×400、2 倍超采样、取消 / 超时、资源预算及 PNG 编码策略保持；预算计入新增 profile 节点。空场景回退与真实 PNG 使用同一浅色背景，不绘制虚构零件。

## 结构树

行高统一为 30 CSS px，虚拟化步长、命中区域和连接线使用同一常量。文字、辅助线、工作主体和琥珀色选择层次更明确，背景用静态渐变薄层提高可读性；不使用背景模糊，不改变画布尺寸，空白区域仍允许视区交互。上下文菜单项仅在打开对应行时构建；Escape 关闭菜单，语义身份、多选和正式命令入口保持。

树仍加载完整节点索引。定向测量未证明整体 React 提交中位数下降，不把延迟菜单构建当作大树分页或容量提升。进一步优化应依据真实交互剖析，而非单次最慢帧。
