# UI 与前端资源验证

当前职责见 [Web 架构](../architecture/current/web.md)。本页记录 2026-11-14 UTC 的定向验证与测量边界，原始图片、JSON 和对照源码写入 `build/agent-work/ui-optimization/`。

## 画面与交互

保留两排工具栏，检查不同桌面宽度下的滚动、分类底边、命令可达性及画布尺寸。文档中心检查缩略图单击打开、勾选独立、紧凑列表、目标文件夹逐层读取、正式复制与移动。管理抽屉检查返回后搜索保留，以及真实创建、修改、密码重置；没有叠加编辑 Modal。选项窗口靠边、按内容高度显示；实体表单隐藏无输入时的空计数 / 禁用编辑和全选，明确操作、单位和创建 / 保存动作，薄壁与起始偏移按需展开。

新浏览器入口为 `ui-refinement-live.spec.ts`。真实 `.env` API、Router、Worker、数据库及 ArtifactStore 覆盖文件夹操作、树展开 / 定义显隐、上下文菜单、悬停、账号保存和批量导入。一次双文件 BREP 导入经过 Vite `/api/realtime`，观察到 11 个任务事件、一条批次完成通知、一个共享 jobs 查询；逐项完成结果仍在任务中心。该事件数是本次样本，不是协议保证。

`visual-studio-live.spec.ts` 另覆盖真实圆角块 / 浅弧 / 鞍面导入、参数化带孔件、预览及八个不同姿态的装配实例。八实例检查私有几何包装、共享 CPU 数组 / GPU 属性、独立 occurrence 路径与移动后的显示。`workbench-layout-live.spec.ts` 覆盖两排布局和矩形变体临时默认值；孔的定向场景覆盖实际拾取、预览、取消、提交、撤销 / 重做与重开。

不将旧的“创建基准面”表单场景视为当前参考元素入口的验收：该场景仍针对旧 DatumEditor，而当前目录使用“参考面”。未执行完整浏览器矩阵、真实硬件 GPU、移动设备或 WebGL context loss / restore。本次没有全量集成层通过声明。

## 状态与请求

优化前只读 Mock 样本为 81 次指针移动、21 次预选变化、21 次相同 Inspector 偏好写入。真实零件新样本为 81 次移动、27 次预选变化、0 次偏好写入；两者用于核对持久化行为，不用于比较指针延迟。Node 另验证 200 次预选和选集 / 工具变化零写入，Inspector 页签真正变化只写一次且仍能读取已有保存值。

目标文件夹样本：打开弹窗只发一次子目录请求，连续进入两层后累计三次，不读取其他分支；路径标签按需读取 breadcrumb，不能把三次子目录请求当作全部 HTTP 数量。正式 copy / move 核对保存的 Folder ID，重命名后再开目标选择器核对路径标签刷新。任务查询场景验证并发观察者请求去重、活动 / 空闲轮询、旧 attempt / completion 拒绝及结果 payload 保留。

## 代码加载

对照提交为 `09374811`。统计生产 `index.html` 的入口和直接 `modulepreload` JS，gzip 为本地 level 6 计算；不是 HTTP 压缩、缓存、解析或首屏可用时间。

| 范围 | 原始 JS：旧 / 新 | 本地 gzip：旧 / 新 |
|---|---:|---:|
| 入口与 HTML 预加载 | 1,571,052 / 1,220,515 B | 495,130 / 387,540 B |
| 工作台主分块 | 751,825 / 460,289 B | 224,232 / 142,318 B |

真实构建 source map 不含 `api/mock-api.ts`。延迟面板仍在首次打开时加载，其共享依赖可能已经由别的入口加载；主分块变小不等于全部工作台下载量按相同比例减少。以重建后的 `bundle.json` 为精确制品清单。

## 表面缓冲

采用原生 OCCT 圆角块 GLB：2,160 个显示顶点、3,348 个三角形。WebGL2 / Chromium / SwiftShader，960×720、DPR 1，比较相同相机、灯光、实例姿态及蓝灰 / 深色 / 彩色材质。首实例另带局部面色，所有包装的分组独立。此资源微基准只绘制表面，不含工作台边线、PMREM、地面阴影或 AO；真实装配另以上述八实例链路验证。

| 实例数 | bufferData 次数：旧 / 新 | 表面属性上传：旧 / 新 | draw：旧 / 新 |
|---:|---:|---:|---:|
| 8 | 24 / 3 | 736,128 / 92,016 B | 9 / 9 |
| 32 | 96 / 3 | 2,944,512 / 92,016 B | 33 / 33 |

初始画面及移除首实例后的画面均逐像素一致；剩余实例无重新上传、无 GL error，最后租约归还后包装计数为零，实际 deleteBuffer 数与各方案上传数相符。计数来自拦截真实 WebGL 调用，只包含 position / normal / index；不声称总显存下降相同比例。单次准备和提交时间保留在 JSON，不用冷态顺序样本声称稳态提速、FPS 或 P95。

## 树与验证入口

`profile-ui.mjs` 将指定提交的原树与当前树放到相同的 5,001 节点宿主中，各三轮、每轮 45 次高亮变化，保存 React Profiler `actualDuration`、可见行数及截图。1280×900、树宽 330 px；回调和外层 Provider 保持稳定，两个版本均关闭 HMR，参考副本只调整导入路径和无会话宿主的临时注册键。本次两侧均渲染 44 行，各 135 个样本，合并中位数为旧 6.9 ms / 新 1.5 ms。这是 React render 耗时，不能称为浏览器完整 commit、帧时间或真实大装配容量。

复现时先完成构建和检查，再串行测量，避免编译和浏览器测量争抢 CPU。安装已有工具即可，不要求下载临时浏览器库。

```bash
mise run check --scope all --jobs 4 --test-jobs 2
OCCCCAD_VISUAL_STUDIO_DIR="$PWD/build/agent-work/visual-studio/fixtures" mise run check --scope geometry --match SurfaceDisplay --jobs 4
node web/apps/cad/scripts/profile-ui.mjs --base 09374811
mise run dev --profile release --jobs 4 --no-web
cd web/apps/cad
pnpm exec playwright test --config playwright.live.config.ts ui-refinement-live.spec.ts visual-studio-live.spec.ts workbench-layout-live.spec.ts
```

Node 场景覆盖共享资源寿命、私有面分组 / BVH、精确 triangle / material index、不可变索引、偏好和任务状态。真实测试清理自己的临时文档 / 文件夹，账号按现有接口禁用；不重置共享数据库、历史或 ArtifactStore。其他已有开发数据不属于本次清理范围。

## 细节与显示复核

2026-11-14，在 Chromium / SwiftShader 中复核短名称、无搜索结构树、轮播和选项弹层。`ui-details.spec.ts` 使用真实页面组件和显式 Mock 鉴权，验证三图循环、鼠标滚轮及惯性抑制、手动重置计时、暂停／恢复、减少动态效果，以及表单输入保持；它不证明服务端鉴权。`ui-refinement-live.spec.ts` 的文档导航场景和 `geometric-feature-live.spec.ts` 的 Geometry tabs 场景另经共享 `.env` 的真实 API / Router / Worker，覆盖短名称选择、几何创建与联动、撤销重做和重开；非完整浏览器矩阵。

宽度宿主放置 1,000 个选项，最长项位于第 1,000 项，触发器宽 96 px，靠近右边界。两种视口均只渲染 10 个选项；键盘可到达并选取最长项，分组选项与自定义字段映射也能完整显示。

| 视口宽度 | 弹层宽度 | 最长项内容宽度 | 文本高度 |
|---|---:|---:|---:|
| 1,440 px | 519 px | 487 px | 21 px，一行 |
| 360 px | 336 px | 304 px | 42 px，两行 |

最长项的 scrollWidth 与 clientWidth 相等。自动化打开及键盘选择墙钟时间约 120／133 ms，包含协议、调度和渲染，单次样本不能当成纯文本度量耗时或稳定延迟指标。Node 场景另核对 10,000 项的末项度量、200 次缓存命中、分组和字体失效。图片与原始结果在 `build/agent-work/ui-details/`。

显示复核使用原生 OCCT 圆角块 `fillet-balanced.glb`，SHA-256 为 `60df6153acf184ec3b4269c6299c23c368ac83d3c2b31f01f7c51c3b53f4e2f1`；2,160 个显示顶点、3,348 个三角形，经生产 VisualRepository / Viewport。1280×900、DPR 1、Chromium / ANGLE SwiftShader，默认工作室设置但关闭网格；相同相机、几何、材质，预热三帧后执行七次显式相同重绘，无 HMR，无计时区内 finish / 像素读回。

| 实例 | 场景对象 | 共享 position 属性数 | 总 draw / 每帧 | 深度 draw / 每帧 | 阴影贴图刷新 / 七帧 | CPU 帧回调中位数 |
|---:|---:|---:|---:|---:|---:|---:|
| 8 | 94 | 1 | 26 | 8 | 0 | 1.2 ms |
| 32 | 262 | 1 | 98 | 32 | 0 | 1.6 ms |

前后截图逐字节相等。深度阶段 CPU 中位数均为 0.3 ms；这些 CPU／提交墙钟数可包含驱动等待，未使用 GPU timer，不能称为独立 GPU 时间。宿主等待两次 RAF 的中位数 31.5／31.8 ms 也不是单帧时间或 FPS。小模型仅支持“阴影缓存生效、表面属性共用、深度仍按实例重复绘制”的结论；尚未验证大型真实装配、原生 GPU、触屏或长期运行。源码入口和后续候选见[显示限制](../architecture/current/visual-studio.md#后续显示优化候选)，原始采样与诊断脚本保存在 `build/agent-work/ui-details/render-profile.json` 和同目录 `profile-render.mjs`。

## 仓库上下文整理

大体积原始测量 JSON 统一到 `tests/test.data/performance/`，解释和链接保留在 validation。两份迁移样本逐字节与原版本相等，共 615,199 B；默认文档扫描减少这些机器数据，总仓库并未因此减少相同字节。重复的浏览器命令搜索步骤归 `browser/command-helpers.ts`，测试通过正式 Command ID 或当前分类区分短名称。删除了已移除的树搜索实现、样式和专用场景，并修正文档中已过期的树搜索和文件夹全量递归描述。有效回归、机器合同、几何语料及架构约定保留；未按文件大小机械拆分引擎或删除测试，也未清空共享开发库。文档字节和入口数只是上下文代理指标，未测量 Agent token 或理解时间收益。

## 图标与深度缓存复核

2026-11-14 UTC，图标以 16／20／24 px 对照，当前目录含 133 个图标，69 个命令改用更具体的图形。`icon-system.spec.ts` 检查全目录 SVG 几何边界、同类特征绑定及 light／dark／mono／disabled 截图；图形边界通过不代表识别速度经过用户研究。结构树独立的状态徽标和同 Revision Feature 元数据参与展示，命令／稳定键／Revision 语义不变。

深度对照复用原生 OCCT 的 `fillet-balanced.glb`：2,160 顶点、3,348 三角形，经生产 VisualRepository／Viewport。Chromium／ANGLE SwiftShader，1280×900、DPR 1，默认工作室灯光和 `#98a8b8` 材质，关闭网格；同一相机、材质、灯光和实例姿态，预热三帧，各模式七次串行显式重绘。对照仅改变缓存开关，双方均使用对象索引，不把差值当作索引独立收益。

| 实例 | 每帧 draw：不缓存／命中 | 每帧深度通道：不缓存／命中 | 画面对照 |
|---:|---:|---:|---|
| 8 | 26 / 18 | 1 / 0 | PNG 逐字节相同 |
| 32 | 98 / 66 | 1 / 0 | PNG 逐字节相同 |

两组各七个命中帧均未直接调用完整 scene 的 traverse／traverseVisible；计数不包含 Three.js 内部矩阵更新和渲染递归，也不表示没有对象或材质检查。CPU 回调与深度阶段墙钟原样保存在 JSON，可含驱动等待；没有独立 GPU timer 或稳定 P95／FPS 结论。少量顺序样本不用于宣称实际导航提速。

同一真实渲染入口另逐项改变灯光、AO 强度、姿态、反射及非均匀缩放、祖先显隐、相机、投影、颜色、position 版本和剖切，分别与强制重画像素核对。颜色、灯光与 AO 强度单独变化命中，其余必要变化重新绘制。透明场景两张必要深度目标都可命中，调整尺寸及加载原生开放浅曲面后仍与强制重画一致；截图命令可在命中帧完成 PNG。Node 覆盖 position／index／drawRange／面组、材质、图层、清屏色、剖切、context restored 事件、异常状态恢复及子树增删／重挂／释放，另核对父标记晚注册时的同帧位置，以及索引路径下斜置约束标记的缩放间距。

原始样本、源 GLB 摘要和 PNG 在 `build/agent-work/icon-depth/`，主要为 `depth-profile.json`、`icon-bounds.json`、`commands-light.png`、`commands-dark.png`。当前 API 与本地编译的图标一致性入口在 `ui-refinement-live.spec.ts`；原生 GPU、大型装配、完整上下文丢失恢复、长期 heap 和人工识别时间未作验收。

```bash
OCCCCAD_VISUAL_STUDIO_DIR="$PWD/build/agent-work/visual-studio/fixtures" mise run check --scope geometry --match SurfaceDisplay --jobs 4
cd web/apps/cad
pnpm exec playwright test icon-system.spec.ts
pnpm exec playwright test --config playwright.studio.config.ts render-cache-render.spec.ts
pnpm exec playwright test --config playwright.live.config.ts ui-refinement-live.spec.ts geometric-feature-live.spec.ts workbench-layout-live.spec.ts
```

原图标比较可额外设置 `OCCCCAD_ICON_REFERENCE` 指向已保存的旧 Catalog JSON；普通回归不依赖 Git 中某个历史提交或该可选输出。

## 浏览器交互与弧线高亮复核

2026-11-15 UTC，`browser-interactions.spec.ts` 经真实 BrowserRouter 与显式 Mock API 检查：文件夹两层往返、筛选返回和刷新恢复、CDP 可信鼠标侧键前进／后退、当前标签去重／关闭替换、跨文档临时表单清理、机构失效地址清理、快速 POP 取消未提交搜索、退出／重新登录的目标页。延迟关闭用 Mock API 注入等待，再通过浏览器后退；请求完成后仍停留于原筛选页，不被迟到的关闭回调跳走。鼠标侧键能驱动同一份历史；Alt 加方向键仅核对页面未 preventDefault，Headless 浏览器可能不执行浏览器外壳的键盘加速键，不能据此声称所有操作系统硬件映射已通过。文档中心的真实右键菜单、右键拖动与 Ctrl+A 禁选、文本框编辑／原生菜单、弹窗标签拖选和登录页例外也由该场景覆盖；本轮未运行真实 API 的完整工作流矩阵。

`highlight-joins.spec.ts` 用生产 LineMaterial / SurfaceDepthPass 比较 192 段平面弧线，白／深／蓝底，1.75／2.25 CSS px 和渲染 DPR 1／2。平面无深度干扰的核心像素原本已较强，该测试主要限制端帽／覆盖率回归；不能把小幅变化当作真实曲面问题的修复证据。

`highlight-native-render.spec.ts` 通过注册的原生 EdgeProfile producer 生成圆柱和圆角块，再经生产 GLB 解码、VisualRepository 与 Viewport 高亮。Chromium／ANGLE SwiftShader，1280×900 CSS px，默认蓝灰材质与同一工作室光照；透视 DPR 1、正交 DPR 1、正交渲染 DPR 2，分别检查 selected／hover，共 12 组。页面设备 DPR 固定 1；DPR 2 对照的实际 drawing buffer 为 2560×1800，PNG 经浏览器合成为 CSS 尺寸，像素分析按实际 PNG 尺寸映射。

对照恢复旧的 MSAA／端帽与半像素线深度容差；当前方案保留有限法线斜率容差。第三张关闭 depthTest 的图只作描边足迹参考，会暴露背面线，不能作为产品显示模式。用显示曲线中点的射线前表面距离筛出可见样本，在每点附近 5×5 PNG 像素内累计橙／青色通道差，再除以同位置参考图；该量只衡量这一夹具的描边保留，不等于物理透明度或通用可见性指标。

| 场景，透视 DPR 1 | 描边保留比 P10：原／当前 | 中位数：原／当前 | 整帧 draw：原／当前 |
|---|---:|---:|---:|
| 圆柱选中 | 0.644 / 1.000 | 0.860 / 1.000 | 5 / 5 |
| 圆角块选中 | 0.598 / 0.813 | 0.789 / 0.977 | 5 / 5 |

固定画面重复帧 PNG 完全相等；加入独立的前景不透明实体后，开启／关闭被挡住的高亮所得 PNG 完全相等。支持法线不增加 draw；边缓冲增加 48 B／段，面轮廓复用已有属性，没有每帧 CPU 法线采样。Node 场景另验证缺失／质量标记／不同端点采样的降级、原折线不变与共享缓冲释放。尚未覆盖硬件 GPU、所有浏览器／鼠标驱动、长时间使用和极近遮挡；缺失支持法线的旧制品只使用原容差，网格与 B-Rep 不一致不属于本次修复。

图像、GLB 摘要、逐场景数值在 `build/agent-work/browser-polish/native/`，平面弧线数值在上一级 `highlight-joins.json`。本轮没有 FPS、GPU 时间或大型装配容量提升声明。

```bash
mise run check --scope web --match interaction-geometry
mise run check --scope web
cd web/apps/cad
pnpm exec playwright test browser-interactions.spec.ts highlight-joins.spec.ts number-input.spec.ts navigation.spec.ts
pnpm exec playwright test --config playwright.edges.config.ts highlight-native-render.spec.ts
```
