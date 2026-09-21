-- Angle-family shortcuts share the ANGLE domain definition.
INSERT INTO occccad.ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES
('assembly-constraints','assembly.parallel','平行','约束两方向平行，可选择同向或反向。','parallel','primary',51,false),
('assembly-constraints','assembly.perpendicular','垂直','约束两方向垂直，保留正向90°或反向270°意图。','perpendicular','primary',52,false)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;
