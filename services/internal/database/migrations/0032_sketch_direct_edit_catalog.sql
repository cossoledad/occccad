-- Presentation changes only; no model/history rows are changed.
DELETE FROM occccad.ui_toolbar_items WHERE command_id IN ('sketch.edit.scale','sketch.edit.quick_trim');
UPDATE occccad.ui_toolbar_items SET group_key='primary' WHERE group_key='more';
UPDATE occccad.ui_toolbar_items SET name='修剪',help_text='点击删除命中区间；没有边界时删除整条曲线。' WHERE command_id='sketch.edit.trim';
UPDATE occccad.ui_toolbar_items SET name='分割',help_text='点击内部位置打断；闭合曲线选两个位置。保留形状并连接新端点。' WHERE command_id='sketch.edit.split';
UPDATE occccad.ui_toolbar_items SET help_text='选择曲线，移动或拖动到目标位置；捕获边界后使用精确交点及连接关系。' WHERE command_id='sketch.edit.extend';
UPDATE occccad.ui_toolbar_items SET help_text='选择两条适用曲线，直接输入半径并预览；Enter 原子提交。' WHERE command_id='sketch.edit.fillet';
UPDATE occccad.ui_toolbar_items SET help_text='选择两条直线，直接输入距离并预览；Enter 原子提交。' WHERE command_id='sketch.edit.chamfer';
UPDATE occccad.ui_toolbar_items SET help_text='直接点选切换标准/构造状态；合法预选整体原子转换。' WHERE command_id='sketch.edit.construction';
