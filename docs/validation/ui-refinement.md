# UI 与前端资源验证

当前职责见 [Web 架构](../architecture/current/web.md)。本页记录 2026-11-14 UTC 的定向验证与测量边界，原始图片、JSON 和对照源码写入 `build/agent-work/ui-optimization/`。

## 画面与交互

保留两排工具栏，检查不同桌面宽度下的滚动、分类底边、命令可达性及画布尺寸。文档中心检查缩略图单击打开、勾选独立、紧凑列表、目标文件夹逐层读取、正式复制与移动。管理抽屉检查返回后搜索保留，以及真实创建、修改、密码重置；没有叠加编辑 Modal。选项窗口靠边、按内容高度显示；实体表单隐藏无输入时的空计数 / 禁用编辑和全选，明确操作、单位和创建 / 保存动作，薄壁与起始偏移按需展开。

新浏览器入口为 `ui-refinement-live.spec.ts`。真实 `.env` API、Router、Worker、数据库及 ArtifactStore 覆盖文件夹操作、树搜索 / 定义显隐、上下文菜单、悬停、账号保存和批量导入。一次双文件 BREP 导入经过 Vite `/api/realtime`，观察到 11 个任务事件、一条批次完成通知、一个共享 jobs 查询；逐项完成结果仍在任务中心。该事件数是本次样本，不是协议保证。

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

Node 场景覆盖共享资源寿命、私有面分组 / BVH、精确 triangle / material index、不可变索引、分支搜索身份、偏好和任务状态。真实测试清理自己的临时文档 / 文件夹，账号按现有接口禁用；不重置共享数据库、历史或 ArtifactStore。其他已有开发数据不属于本次清理范围。

本轮执行：全仓 unit scope 通过，两个可选缩略图图库 / 测量用例因未设置其专用输出参数跳过；最终 Web scope 的 167 个场景及生产构建通过。定向真实浏览器共十项通过（含到平面范围、失效支撑修复与撤销 / 重做）；旧“基准面”创建场景未作为当前入口验收。额外 Mock 冒烟确认 API 模式为 Mock、无真实 `/api` 请求，视口与延迟建模面板正常。浏览器读取已挂载组件的引擎引用，不靠导入另一个 HMR 模块或移动相机来安装探针。

对照图入口为 `build/agent-work/ui-optimization/comparison.html`。普通删除清理本次遗留的 11 个临时文档并逐一读回 404；通过 UI 流程创建的账号已禁用，文件夹由各场景的 finally 按原 ID 删除。删除进入正常回收站，保留既有历史和制品，不等同于物理清空数据库或桶。
