-- View-only command shared by Part, Product and Sketch workbenches.
INSERT INTO occccad.ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES ('view-navigation','view.normal','法线视图','选择基准面或实体平面后正对该面，保留显示比例。','top-view','primary',60,false)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;
