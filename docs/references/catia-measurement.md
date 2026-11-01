# CATIA 几何测量参考

本页区分手册事实、视觉观察与 occccad 的设计选择。通过本机 B33 英文资料 `control/CfyEnglishC2.viewdoc` 定位 `online/cfyugmeasure_C2/`，并使用 `catia_manual` MCP 的 describe → infrastructure search → fetch/分页读取原文。没有运行 CATIA、自动化接口或其样例模型。

MCP 简化来源：库 `English`，library ID `6d5aed22-2df7-427c-83ae-0b4926af15ef`，index revision `9058175bec4c4d87f0f646ae`；支持文本检索/读取，不支持图片或模型。下面路径是返回的 SourceLocation.document.path，不能把检索摘要当成完整原文。

| SourceLocation 路径（均位于 `online/cfyugmeasure_C2/`） | 原文可确认行为 |
|---|---|
| `cfyugmeasurebetween03.htm` | Measure Between 用于点、边、面、整组件；依次选择 1/2，视区画最短距离向量。Between、Chain、Fan 分别是独立配对、末项续接、固定首项。Part/Product 默认各用其活动坐标系。Keep measures 将对象加入树。 |
| `cfyugmeasuredialogbox.htm` | 选择模式独立于测量类型，包括实际/无限几何、拾取点、点/边/面/组件过滤、轴等。圆心与圆柱轴属于可识别语义。拾取点依赖剖分，不保证位于精确几何且不关联。Exact、Approximate 和 exact-else-approximate 区分计算来源，近似通常带 `~`。 |
| `cfyugmeasurecustomize.htm` | 可分别控制几何区和对话框中的结果、选择名称、坐标点及分量；最短、最大、单向最大距离互斥。最大距离的含义包含垂直偏差及方向性，不能解释成任意两点最远距离。 |
| `cfyugmeasurebetween02.htm` | 曲线角度涉及交点与切线；多交点时选择位置参与交点判定。只有两平面才有相应面间解析角度。拖动角度线可改为补角，结果与知识参数同步。不能从任意曲面的法向或圆的轴向推断用户所需夹角。 |
| `cfyugmeasureproperties.htm` | Measure Item 依对象类型给出精确或近似属性，支持其他轴系。保留对象可关联更新、参与参数及公式。视区显示面积等结果。 |
| `cfyugmeasureitmedialogbox.htm` | 点/边/面/组件过滤；三点角度的第二点是公共顶点，可以显示补角。识别圆柱/球半径和椭圆。测量工具之间可以切换。 |
| `cfyugmeasurecustomizeitem.htm` | 边显示长度/端点/方向，圆弧有半径和角度。面有面积/形心/周长，平面有方程。端点顺序固定后需要保持稳定，不能随显示曲线反向而随意换序。 |
| `cfyugmeasurecursors.htm` | 悬停高亮帮助区分几何类型；Between 光标用 1/2 表达当前选择阶段。 |
| `cfyugmeasureassociative.htm` | 修改/移动来源后，树对象变为未更新；可局部/强制更新。自动更新有单独选项，默认关闭；重开文档不会天然执行更新。来源无法解析属于无效测量，和未更新不同。并非所有选择模式都可关联。 |

主要页面内容哈希：between03 `8dbf5016dcc9dfeadec2a5ffa4bf3c674eccaad63d5812ea078d020f92a1ab26`；dialogbox `0c4567fa8fdfefb23264c283bf62ee6d0b9e3c992700acf76961d597591891f0`；customizeitem `6586f59224a1fbe36d8a835853f599eb4f5717e831d5dbedea52531e888ab40f`。重新连接 MCP 时以 SourceLocation 重新 resolve，内容变化应重新读取。

## 视觉依据与取舍

另外直接查看本机同目录 `images/gameasurebetween03.gif`、`gameasureitem01.gif`、`gameasureitem08.gif`。可观察到所选对象橙色轮廓、绿色见证点/测量线、带底色的读数及到对象的引线；局部坐标结果采用多行 XYZ。图片只能证明手册插图的呈现，不能证明运行中的拾取容差、遮挡策略、拖动手感或计算性能。

occccad 采用共同“测量”工具栏、统一命令表单、明确字段 1/2 和三点顶点、精确 B-Rep 来源、显式保留/手动更新。采用与现有深色视区匹配的青色线、浅色文字与深色底牌，文字和箭头保持 CSS 像素尺寸；世界坐标锚点随相机投影。标签位置属于本地呈现状态。

第一条闭环不引入 CATIA 的完整选择/计算模式菜单：没有网格近似兜底、截图拾取点、任意曲线切角、补角拖动、最大垂直偏差、测厚、其他轴系或公式驱动。它们与当前算法的语义差异必须保留，不能用同名但不同定义的数值填充。物理质量、密度、惯量明确不在本次范围。已实现合同及未解决问题归[几何测量](../architecture/current/geometric-measurement.md)。
