# 对象属性与全局选项

对象数据用树右键“属性…”或 `Alt+Enter`，应用设置用顶部齿轮“选项”。对象属性写 Revision；全局选项保存于当前浏览器，作用于所有文档。外观模型和 STEP 边界见[颜色材质](surface-appearance.md)。

## 入口与菜单

- **唯一可见的全局设置入口**是全局标题栏“选项”，在文档中心和所有工作台均可用，执行 `app.options`；命令搜索投影同一命令。右下角保留状态、单位/导航提示和当前工具动作。工具自己的“工具选项”与应用设置分开命名。
- **视区右键没有菜单**，原菜单组件、候选列表、事件回调和点击检测均已删除；保留浏览器原生菜单抑制、右键旋转和工具取消行为。选中面后可用 `Alt+Enter` 查看属性。
- **树菜单**按通用操作、显示/隐藏、删除/复制别名、`xxx 对象` 子菜单、底部“属性…”分组。定义编辑、工作对象、抑制、引用模式和重连等放在对象子菜单，只投影实际支持的能力。多选只显示可针对选集执行的操作，不把“编辑点击的单个对象”混入批量操作。
- 独立“重命名”“颜色材质”命令、注册和表单入口已移除。名称和外观统一在属性中；机构/关节/驱动/模拟名称复用属性表单，提交仍经既有 `SAVE_KINEMATICS`，不改产品约束。

顶部入口的选择是本软件的取舍：文档中心也需要配置新建默认值，顶部全局位置比仅工作台右下角更一致；无需重复增加“场景属性”按钮或工具栏分类。

## 对象属性

“基本 / 图形 / 信息”固定打开时的目标和基准 Revision。相机与选择变化不切换编辑目标；多选要求同一写入文档和基准版本，混合字段只写实际改动。

| 目标 | 可修改项 | 所有权 |
|---|---|---|
| 根文档 | 名称、描述、默认外观 | 当前 Part/Product |
| Body | 名称、整体与边线外观 | 原始 Part；装配中可仅覆盖当前 occurrence 外观 |
| 面 | 颜色、透明度、粗糙度、金属度、选择性 | 稳定拓扑 anchor；也可为宿主 occurrence 覆盖 |
| 特征/草图、用户基准面/轴 | 名称 | 原始 Part；标准基准面不可改名 |
| 组件实例 | 名称和外观 | 直接实例属当前 Product；嵌套名称为宿主别名 |
| 机构、关节、驱动、模拟 | 名称 | 当前 Product 的机构定义 |

作用范围、写入文档和版本明确显示。Inspector 保持只读；历史/只读上下文不能写入。透明度显示为 0–100%，模型保存 opacity=1−透明度；字段独立继承/自定义，整体修改保留分面属性。

外观通过可取消的防抖 PreviewCommand 返回正式投影，不创建 Revision。确定原子提交，取消清除预览；迟到响应不能覆盖新草稿。冲突保留修改，可重新加载基线；实例删除/替换、面的 GeometryKey 改变时须重新选择，不按旧面号重绑。文档名称/描述补偿与 live 摘要在同一 CAS 事务完成，历史查看不重命名 live 文档。

## 全局选项

一个左侧分类、右侧内容的对话框，页内少量标签和折叠高级项；“恢复当前页默认设置 / 取消 / 确定”统一收口。当前对话框草稿临时预览，取消不保存；确定一次性保存完整快照。切换文档和刷新复用同一全局设置，不存在文档级场景覆盖。旧 UI preferences 升级保留导航、捕获和单位等既有偏好，场景使用新的全局默认，不从某个文档任意挑选旧设置。

| 分类 | 当前设置 |
|---|---|
| 场景 | 深蓝/明亮/石墨配色；上下渐变/纯色；地面位置、网格、阴影；光照预设及方向 |
| 显示 | 正常/线框、边线、交点、基准面/轴/坐标系、草图网格 |
| 新建对象 | 新建零件实体表面颜色、粗糙度、金属度 |
| 导航与捕获 | occccad/CATIA/SOLIDWORKS 手势、旋转球、捕获角色与预设 |
| 单位 | 全局默认显示/新建输入单位；保留明确标注的当前文档显示单位覆盖 |

`app.options` 是全局 UI Command；场景和导航设置不产生模型 Revision、不进入模型 Undo/Redo。当前存储是浏览器本地设置，不承诺账号跨设备同步或项目级共享场景。对话框显式使用统一 Modal 层级，让其 Select/ColorPicker 子弹层正常位于面板上方。

### 场景与显示规则

渐变固定于屏幕上下端点，纯色使用同一颜色；不随旋转、缩放或平移混入其他色调。网格自动按背景明暗选择线色，主线在宽度与强度上高于细线，支持手动颜色/强度；保留自动/指定间距和独立世界网格。辅助网格只用于观看，草图捕获与网格显示独立。

自动地面从各文档可见、未消费的权威实体世界包围盒计算 minZ，并加全局偏移；不使用基准、失败回退或临时/历史制品。世界模式固定 Z=0，指定/锁定高度是全局绝对高度。拖拽/运动预览期间冻结包围盒，接受模型后更新；相机旋转不改变地面高度。空场景回到 0。当前高度/网格间距是活动视口的只读信息。

光照复用已有程序环境和四盏灯，保留哑光默认；可选地面阴影，透明物体不投射实体阴影。没有图片/HDR 资源导入、物理折射/透明阴影、任意倾斜地面或共享视图预设。

### 新建默认值是创建快照

`creationDefaults.solid` 是可扩展的新建对象类别，当前只交付实体；后续曲面等类别应在此处独立扩展，不增加零散入口。普通新建 Part 和 Product 中新建 Part 均在创建请求携带 `defaultAppearance`；服务端验证为整体 `SurfaceStyle`，固化到新 Part 的首个 Revision，与初始事务一起保存。

后续新建 Body/特征继承其所属 Part 的默认外观。改变全局新建默认值不会重染现有文档、导入模型、复制件或共享实例；Product 本身不注入零件实体默认值，避免覆盖引用 Part 的外观。修改已存在文档的默认值应使用对象“属性”。

## CATIA 参考与源码

本机 B33 文档根：`/mnt/c/UserApp/3D/DS/Dassault Systemes/B33doc/English/`。先从 `control/BasEnglishC2.viewdoc` 定位，具体页：

- `online/basug_C2/basugwd0100.htm`、`basugcu0200.htm`：Tools → Options 统一设置分类；`online/basil_C2/basilAboutSettings.htm` 区分临时与永久设置。
- `online/basug_C2/basugbt0602.htm`：单选 `xxx object` 与多选 `Selected objects` 的对象命令组织。当前只放已实现操作，并按用户要求不提供视区右键菜单；不是逐项复制 CATIA。
- `online/basug_C2/basugbt0706.htm`：对象 Graphic 属性；`online/pstug_C2/pstugbt0307.htm`、`pstugbt0341.htm`：实例与定义区分及继承。
- `online/basug_C2/basugbt1305.htm`：初始地面在模型底部、允许移动；不能据此声称 CATIA 每帧贴底。当前自动/世界/指定规则是本项目明确契约。

| 修改范围 | 从这里进入 |
|---|---|
| 全局命令、草稿与分类 | `features/preferences/global-options-{center,command,model}.tsx/ts`、`scene-options-fields.tsx`、`state/ui-preferences.ts` |
| 对象表单/作用域 | `object-properties-dialog.tsx`、`property-target.ts`、`tree-context-menu.ts` |
| 背景/网格 | `cad-background.ts`、`infinite-ground-grid.ts`、`scene-settings.ts`、`cad-shader-library.ts` |
| 创建快照 | `graphic_properties.go`、`CreateDocument`、`CreatePartComponent`、`creation_appearance_test.go` |
| 验证 | `global-options.scenario.mjs`、`browser/properties.spec.ts`、`properties-live.spec.ts`、`navigation.spec.ts`、`motion-workbench.spec.ts`；执行证据见[验证记录](../../validation/README.md) |
