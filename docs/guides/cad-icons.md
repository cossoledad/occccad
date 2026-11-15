# CAD 图标设计与扩展

工具栏、结构树和上下文菜单共用 `CadIcon`。命令通过稳定的 `iconKey` 选择图形，图标不包含命令行为、对象身份或求值状态。唯一绘图源为 [icons.json](../../services/internal/workbenchconfig/icons.json)，命令绑定归 [catalog.json](../../services/internal/workbenchconfig/catalog.json)。

## 视觉语法

| 层次 | 规则 |
|---|---|
| 网格 | `20 × 20` 坐标空间，主要轮廓尽量放在 `1…19` 内；工具栏／树通常显示为 18 CSS px，检查 16／20／24 px |
| 笔画 | 主线 1.5，动作线 1.7；圆端点、圆连接，无文字字体或外部位图依赖 |
| 几何母形 | 实体用闭合体，曲面用无侧壁的开放片，曲线用单轨迹；Part 为文档＋块体，Body 为块体，Product 为关联部件 |
| 动作 | 拉伸方向、扫掠路径、放样截面、切除空腔、圆角圆弧、倒角斜边必须在小尺寸下可辨；必要时只画关键轮廓，不强求完整微型零件 |
| 辅助信息 | 原轮廓、轴线和构造关系用弱虚线；端点／控制点用少量实心节点；浅填充只帮助读形 |
| 状态 | 主图表达类型，右下角独立徽标表达失败、抑制或更新状态；颜色不是唯一编码，不把状态烘焙到几何图标里 |
| 配色 | 中性轮廓配少量族色；选中／禁用时继承交互颜色，深色树优先保证对比；禁止用固定白色块擦除图形 |

同一操作跨入口复用同一图标；不同操作不能只换颜色。实体拉伸和曲面拉伸保留相同方向标记，但母形不同。圆角与半径尺寸分别是修改轮廓和测量标记，不共用图形。

## 数据与渲染

`icons.json` 的 `symbols` 保存可复用的叶节点图形，`icons` 保存 `family`、最多八个 `parts` 及可选 `elements`。组合不递归，也不引入任意 SVG/脚本解释器。Go 的 `compileIcons` 和前端的 `compileIconCatalog` 把组合展开为现有 `IconDefinition.elements`；API 仍返回平铺的图标定义，未改变 Catalog schema。

支持 `path / circle / rect / line / polyline / polygon / ellipse`，属性沿用服务端及渲染器白名单。使用以下 CSS 角色，不在 JSON 中写局部颜色、笔画或变换：

| `className` | 含义 |
|---|---|
| 无 | 主轮廓 |
| `icon-face` | 同色低透明度面填充，无描边 |
| `icon-operation` | 操作的关键轮廓／方向 |
| `icon-node` | 实心端点或控制点 |
| `icon-guide` | 原轮廓、轴线、辅助关系 |

允许的族为 `solid / surface / curve / sketch / constraint / assembly / reference / view / status`。族色是界面辅助编码，不覆盖模型材质。所有元素使用同一坐标系；可复用部件的摆放必须在设计时确定。

视口光标同样读取此图形源：`navigation-orbit/pan/zoom/roll` 属于 `view` 族，分别表示旋转、平移、缩放和视轴滚转。旋转用块体外的一条回转箭头，滚转用平面圆箭头，避免多个重叠环在小尺寸下混成一团。`builtin-icons.ts` 只编译图形，不加载命令运行时；`icon-elements.ts` 共享渲染白名单，并将 CSS 角色转换为独立 SVG 的显式笔画。光标采用 32 CSS px、居中热点与透明边缘，深浅背景各取产品配色；需要修改图形时仍只编辑 JSON，再检查实际光标大小。

## 增加一个图标

1. 先查已有图标和母形，优先复用相同语义。使用可搜索的稳定英文 key，不根据中文名称、树行位置或数组下标生成 key。
2. 在 `icons` 中添加定义；确有多处复用的轮廓再放入 `symbols`。下例是一个成员片段，动作路径须按实际功能设计：

```json
"surface-example": {
  "family": "surface",
  "parts": ["sheet-patch"],
  "elements": [
    {"tag": "path", "attributes": {"d": "M7 9h6m-3-3v6", "className": "icon-operation"}}
  ]
}
```

3. 在命令定义中指定 `iconKey`。树中的特征图标优先通过正式 Feature 类型／几何类型查同一命令定义；特殊对象在 `tree-node-descriptors.tsx` 中登记。拥有文档及 Revision 匹配时才读取 Feature 几何类型，没有对应快照时保留通用图标，不猜测名称。
4. 打开开发页面 `/browser/icon-studio.html`，检查 light、dark、mono、disabled，尤其是 16 px 和相邻命令的差异。新图标自动加入完整目录，不需要维护第二张图标清单。

配置由 API 构建时嵌入。更新定义后重新构建并重启开发服务；仅 Vite 热更新不能替换运行中 API 的图标目录。不要为图标增加独立 HTTP 服务或手工维护一份服务端／前端副本。

## 验证入口

```bash
mise run check --scope services --match IconComposition
mise run check --scope web --match icon-catalog
cd web/apps/cad
pnpm exec playwright test icon-system.spec.ts
```

Go／Node 检查组合、引用与接口形状，浏览器检查所有图形边界和多种显示状态；真实 API 对照由 `ui-refinement-live.spec.ts` 的 icon catalog 场景覆盖。图形不同和边界通过不等于识别速度已提升，仍需在实际工具栏／树中观察误选与寻找成本。涉及布局的变更还需验证两排、矩形临时默认值、按钮点击范围和窄屏命令可达性。
