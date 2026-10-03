-- Variants remain ordinary commands. The browser groups presentation and remembers
-- the last explicit choice for its session; no model/history state is changed.
DELETE FROM occccad.ui_toolbar_items WHERE command_id='sketch.normal';
UPDATE occccad.ui_toolbar_items SET name='内接多边形',help_text='中心与构造圆半径；多边形边与圆相切，边数 3–50。' WHERE command_id='sketch.polygon';
INSERT INTO occccad.ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES('sketch-profiles','sketch.polygon.circumscribed','外接多边形','中心与构造圆半径；多边形顶点位于圆上，边数 3–50。','polygon','primary',31,true)
ON CONFLICT(toolbar_id,command_id) DO UPDATE SET name=EXCLUDED.name,help_text=EXCLUDED.help_text;
UPDATE occccad.ui_toolbar_items SET group_key='variants:'||CASE
 WHEN command_id IN('sketch.spline','sketch.spline.control') THEN 'spline'
 WHEN command_id IN('sketch.rectangle','sketch.rectangle.center','sketch.rectangle.oriented') THEN 'rectangle'
 WHEN command_id IN('sketch.circle','sketch.circle.three_point') THEN 'circle'
 WHEN command_id IN('sketch.arc','sketch.arc.three_point','sketch.elliptical_arc') THEN 'arc'
 WHEN command_id IN('sketch.ellipse') THEN 'ellipse'
 WHEN command_id IN('sketch.polygon','sketch.polygon.circumscribed') THEN 'polygon'
 WHEN command_id LIKE 'sketch.edit.spline_%' THEN 'spline_control'
 WHEN command_id IN('sketch.dimension.linear','sketch.constraint.horizontal_distance','sketch.constraint.vertical_distance') THEN 'linear'
 WHEN command_id IN('sketch.constraint.radius','sketch.constraint.diameter','sketch.constraint.major_radius','sketch.constraint.minor_radius') THEN 'axis'
 ELSE 'angle' END
 WHERE command_id IN('sketch.spline','sketch.spline.control','sketch.rectangle','sketch.rectangle.center','sketch.rectangle.oriented','sketch.circle','sketch.circle.three_point','sketch.arc','sketch.arc.three_point','sketch.ellipse','sketch.elliptical_arc','sketch.polygon','sketch.polygon.circumscribed','sketch.edit.spline_insert','sketch.edit.spline_delete','sketch.edit.spline_close','sketch.edit.spline_control','sketch.dimension.linear','sketch.constraint.horizontal_distance','sketch.constraint.vertical_distance','sketch.constraint.radius','sketch.constraint.diameter','sketch.constraint.major_radius','sketch.constraint.minor_radius','sketch.constraint.angle');

-- Arc variants belong to one presentation category.
UPDATE occccad.ui_toolbar_items SET toolbar_id='sketch-profiles',sort_order=37 WHERE command_id='sketch.arc';
