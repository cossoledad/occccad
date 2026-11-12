# 建模展示与视口截图

登录页的三张展示图来自 occccad 的真实参数化模型，由当前 API、Geometry Worker 与浏览器视口生成。图片使用实际材质和场景显示，没有外部模型或生成式图片。

| 模型 | 建模内容 | 图片 |
|---|---|---|
| 径流叶轮 | 旋转轮盘与轮毂、三组样条截面放样、11 片圆周阵列、轴孔键槽 | [impeller.png](../../web/apps/cad/public/showcase/impeller.png) |
| 过渡壳体 | 椭圆到圆的偏心多截面放样、双开口抽壳、法兰、颈环和安装孔 | [enclosure.png](../../web/apps/cad/public/showcase/enclosure.png) |
| 法兰联轴器 | 旋转法兰、圆周孔阵列、倒角、螺栓螺母；14 个实例，固定、同轴及平面间距约束 | [assembly.png](../../web/apps/cad/public/showcase/assembly.png) |

这些是展示建模能力的简化示例，不包含制造设计校核。螺纹未建模；联轴器只约束两片法兰，紧固件按配方放置。配方校验法兰间距和约束求值状态，不替代整机干涉分析或工程验收。

## 保存当前视口

在视口底部点击相机图标，或搜索工具 **保存视口截图**，浏览器直接下载以文档名称命名的 PNG。

截图包含当前模型、材质、背景、阴影、选择高亮、几何预览与可见的几何测量标注。调整视角、显隐和场景后再截图即可；工具条、弹窗和输入控件不进入图片。分辨率等于视口实际渲染缓冲的尺寸，沿用当前设备像素比及渲染器上限。

此操作不提交模型命令，不改变 Revision、相机或选择。它在视口完整绘制后复制画面，并合成同一帧的测量 SVG；不启用长期保留 WebGL 缓冲。关闭视口或图形上下文丢失会结束未完成的截图请求。

## 重新生成展示图

先启动[真实开发环境](../development/environment.md)，在仓库根目录执行：

```bash
mise exec -- pnpm --dir web/apps/cad showcase
# 仅重新取景 / 生成一个示例
mise exec -- pnpm --dir web/apps/cad showcase --model=impeller
```

[生成配方](../../web/apps/cad/scripts/generate-showcase.mjs)读取当前 `.env` 的账号，复用已有 Chromium，在当前开发资源中创建“展示模型”文件夹和正式文档。模型通过 Domain Commands 建立，草图连接使用正式约束，修饰特征使用绑定的持久选择；颜色和源草图显隐也经过正式命令。模型创建失败或无实体结果时不发布该模型的新截图。

每张图片都由工作台的截图按钮下载到 `web/apps/cad/public/showcase/`。当前取景为正交投影和无网格的浅色场景，图片保持渲染输出；登录页只负责展示和手动切换。

临时文档身份、命令与 Revision 回执、精确实体体积、拓扑统计和取景信息保存在 `build/agent-work/model-showcase/scenes.json`；完整工作台截图保存在同目录。再次执行时复用仍存在的文档，删除对应 manifest 条目后可重建该示例。清除开发数据库后，配方可重新建模；不要把临时 DocumentId 写入前端资源。

## 界面文案与布局

品牌标识采用开放的等轴测轮廓和折返内线，复用同一 SVG 作为页面标识与 favicon。登录页用真实模型承载展示，表单保留明确字段和单个主要动作。

文档中心的文件夹支持单击和键盘进入，卡片名称可直接打开文档；多选后才显示批量操作。用户管理保留批准或编辑作为每行主要动作，其余操作放在更多菜单。说明性操作知识放在指南中；界面保留字段标签、实时进度、失败原因、会丢失信息的结果警告和不可撤销操作的确认。

实现入口：[登录](../../web/apps/cad/src/features/auth/auth-screen.tsx)、[文档中心](../../web/apps/cad/src/features/documents/document-center.tsx)、[用户管理](../../web/apps/cad/src/features/admin/admin-drawer.tsx)、[截图生命周期](../../web/apps/cad/src/viewport/viewport-snapshot.ts)。真实浏览器检查入口为 `browser/brand-library-live.spec.ts`，其中截图检查覆盖原生 DPR 1/2、测量标注和模型状态不变；结果以实际执行报告为准。
