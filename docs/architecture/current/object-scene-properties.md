# 统一对象属性与场景属性

当前通过树和视区右键“属性…”、`Alt+Enter` 编辑对象，通过视区设置或对象属性进入“场景属性…”。前者写文档 Revision，后者保存当前浏览器的个人显示偏好。外观模型和交换边界见[颜色材质](surface-appearance.md)。

## 对象属性

对话框使用“基本 / 图形 / 信息”分页，固定打开时的目标和基准版本。视区仍可旋转和缩放，选择改变不会悄悄切换属性目标。多个对象须有同一写入文档和基准 Revision；不同字段显示多个值，只写用户实际修改的字段。

| 目标 | 可修改项 | 所有权 |
|---|---|---|
| 根文档 | 名称、描述、默认外观 | 当前 Part/Product |
| Body | 名称、整体与边线外观 | 零件定义；装配中也可仅做当前 occurrence 外观覆盖 |
| 面 | 颜色、透明度、粗糙度、金属度、选择性 | 稳定拓扑 anchor；可为宿主 occurrence 覆盖 |
| 特征/草图、用户基准面/轴 | 名称 | 原始 Part；标准基准面不可改名 |
| 组件实例 | 实例名称和外观 | 直接实例名称属于当前 Product；嵌套名称为宿主显示别名 |

“仅当前实例”与“当前零件定义”明确显示写入范围；原始文档链接在有未保存修改时禁用。只读或历史上下文可以检查属性，不能写入；现有 Inspector 保持只读。透明度以 0–100% 显示，正式模型保存 opacity=1−透明度。字段“继承 / 自定义”独立控制，颜色来源可查看，整体修改不会隐式清除分面赋色。

外观输入经防抖、可取消的服务端 PreviewCommand 返回同一投影；候选不创建 Revision。确定后一个正式命令原子提交名称和外观，取消/关闭清除预览。过期响应不能覆盖新草稿。提交冲突保留草稿，“加载最新版本并保留修改”只保留用户改动，重新读取基线；实例删除/替换或面的 GeometryKey 改变时要求重新选择，不按旧面号重绑。

文档名称/描述的冻结 `metadata` 参与 ChangeSet 补偿，同一短 CAS 事务更新 live 文档摘要；历史查看不重命名 live 文档。文档中心 PATCH 也携带客户端 `versionId` 并走正式属性命令。稳定身份不依赖名称；复制 Product 时只重置宿主属性路径的根所有权，保留引用零件身份。

视区右键仅在静止的右键单击后打开，导航拖拽和编辑工具不被菜单截断。遮挡对象可从“选择其他对象”候选列表选择，再查看属性。

## 场景属性

场景按文档保存于 UI preferences，实时预览，确定保存、取消恢复；不进入 Revision、Undo/Redo、模型坐标或约束求解。当前是浏览器本地偏好，不是服务端共享场景，也不跟随 STEP 导出。

| 设置 | 当前行为 |
|---|---|
| 自动地面（默认） | 从可见、未消费的权威实体世界包围盒取 minZ，加用户偏移；空场景回到 0；不使用基准面、失败回退和临时/历史制品 |
| 世界地面 | 固定 XY / Z=0 |
| 指定/锁定高度 | 显式高度；自动模式可锁定或重新贴合 |
| 运动与拖拽 | 冻结包围盒，避免预览抖动；接受新模型后更新，不按相机旋转改变地面高度 |
| 网格 | 地面网格、独立世界基准网格；自动/指定间距、颜色、强度；与草图网格和捕获独立 |
| 光照 | 工程/柔和/轮廓预设，亮度、环境/侧向补光、反射强度、世界方位/仰角 |
| 背景与阴影 | 跟随主题/纯色/渐变；可选地面柔和阴影；复用现有程序环境，不加载图片/HDR 文件 |

当前地面高度与网格间距只读展示。地面是观看辅助，不成为可选几何或建模参考。透明物体不投射实体阴影；未实现物理透明阴影、折射、任意倾斜地面、共享视图预设或按面独立边线样式。

## CATIA 对照与取舍

本机 B33 文档位于 `/mnt/c/UserApp/3D/DS/Dassault Systemes/B33doc/English/`，具体参考页：

- `online/basug_C2/basugbt0706.htm`：对象 Properties/Graphic 中的颜色、透明度、边线等。当前采用分页、批量混合值、字段继承和实时取消预览。
- `online/pstug_C2/pstugbt0307.htm`、`pstugbt0341.htm`：实例名称与定义属性、图形继承。当前明确区分宿主 occurrence 与共享 Part，继承规则以本项目上述契约为准。
- `online/basug_C2/basugbt1305.htm`：地面初始位于模型底部并允许手动移动；该页不能证明 CATIA 每帧动态贴地。本项目明确提供自动、世界和指定模式。
- `online/basug_C2/basugbt1701.htm`：独立光照控制。当前用预设和少量直接参数，场景与文档属性有清晰保存范围。

网站对照：[Onshape 外观](https://cad.onshape.com/help/Content/PartStudio/customizing_appearances.htm)、[Fusion 场景](https://help.autodesk.com/cloudhelp/ENU/Fusion-Render/files/GUID-0A159555-E362-4150-AE4F-E9E1308FEF40.htm)、[Three.js 透明排序限制](https://threejs.org/manual/pages/transparency.html)。这些是设计参考，不代表当前软件拥有对方的完整能力。

## 源码与验收入口

- `object-properties-dialog.tsx`、`property-target.ts`：批量草稿、作用域、版本刷新；`scene-properties-dialog.tsx`、`scene-settings.ts`：场景草稿和归一化。
- `cad-viewport-engine.ts`、`scene-presentation.ts`：显示更新、权威包围盒、地面及光照资源生命周期。
- `properties_test.go`：正式槽、两步补偿、元数据原子写入/幂等/过期版本、复制与替换所有权。
- `browser/properties.spec.ts`：Mock 菜单、快捷键和个人设置持久化；`browser/properties-live.spec.ts`：真实 API/Worker、拾取、属性历史、取消、冲突恢复与地面；用例存在不等于已执行通过，运行证据见验证记录。
