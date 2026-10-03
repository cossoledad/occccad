-- Move owns translation and rotation; this changes presentation only.
DELETE FROM occccad.ui_toolbar_items WHERE command_id='sketch.edit.rotate';
UPDATE occccad.ui_toolbar_items SET name=CASE command_id
 WHEN 'sketch.edit.delete' THEN '删除'
 WHEN 'sketch.edit.copy' THEN '复制'
 WHEN 'sketch.edit.move' THEN '移动'
 WHEN 'sketch.edit.offset' THEN '偏移'
 WHEN 'sketch.edit.complement' THEN '补弧'
 WHEN 'sketch.edit.fillet' THEN '圆角'
 WHEN 'sketch.edit.chamfer' THEN '倒角'
 WHEN 'sketch.edit.construction' THEN '构造线'
 ELSE name END WHERE toolbar_id='sketch-edit';
UPDATE occccad.ui_toolbar_items SET help_text='使用手柄移动或旋转；实时求解并保留现有约束。' WHERE command_id='sketch.edit.move';
